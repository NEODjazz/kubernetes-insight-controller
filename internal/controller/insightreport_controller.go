package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"k8s-insight-controller/api/v1alpha1"
	"k8s-insight-controller/internal/analysis"
	"k8s-insight-controller/internal/llm"
)

type InsightReportReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	HTTPClient *http.Client
}

func (r *InsightReportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var report v1alpha1.InsightReport
	if err := r.Get(ctx, req.NamespacedName, &report); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	interval := parseInterval(report.Spec.Interval)
	if err := r.deleteExpiredSnapshots(ctx, &report); err != nil {
		return ctrl.Result{}, err
	}

	analyzer, err := r.analyzerFor(&report)
	if err != nil {
		return r.updateStatus(ctx, &report, interval, "LLMConfigError", err.Error(), "", "")
	}

	analysisStartedAt := time.Now()
	collector := analysis.Collector{Client: r.Client, HTTPClient: r.HTTPClient}
	snapshot, err := collector.Collect(ctx, analysis.CollectOptions{
		Namespaces:       report.Spec.Namespaces,
		PrometheusURL:    report.Spec.PrometheusURL,
		IncludeEvents:    report.Spec.IncludeEvents,
		MaxMetricSamples: report.Spec.MaxMetricSamples,
	})
	if err != nil {
		return r.updateStatus(ctx, &report, interval, "CollectError", err.Error(), "", "")
	}

	snapshotJSON, err := analysis.SnapshotJSON(snapshot)
	if err != nil {
		return r.updateStatus(ctx, &report, interval, "SnapshotError", err.Error(), "", "")
	}

	recommendations, err := analyzer.Analyze(ctx, snapshotJSON)
	if err != nil {
		return r.updateStatus(ctx, &report, interval, "LLMError", err.Error(), snapshotHash(snapshotJSON), "")
	}

	analyzedAt := time.Now()
	if err := r.createSnapshot(ctx, &report, analyzedAt, analyzedAt.Sub(analysisStartedAt), recommendations); err != nil {
		return r.updateStatus(ctx, &report, interval, "SnapshotPersistError", err.Error(), snapshotHash(snapshotJSON), recommendations)
	}

	logger.Info("updated insight report", "name", report.Name)
	return r.updateStatus(ctx, &report, interval, "Ready", "Analysis completed", snapshotHash(snapshotJSON), recommendations)
}

func (r *InsightReportReconciler) analyzerFor(report *v1alpha1.InsightReport) (llm.Analyzer, error) {
	provider := strings.TrimSpace(report.Spec.LLMProvider)
	if provider == "" {
		provider = llm.ProviderAzureOpenAI
	}

	switch provider {
	case llm.ProviderAzureOpenAI:
		if report.Spec.AzureEndpoint == "" || report.Spec.AzureDeployment == "" {
			return nil, fmt.Errorf("azureEndpoint and azureDeployment are required when llmProvider is %q", llm.ProviderAzureOpenAI)
		}
		apiKey := os.Getenv("AZURE_OPENAI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("AZURE_OPENAI_API_KEY environment variable is not set")
		}
		return llm.AzureOpenAI{
			Endpoint:     report.Spec.AzureEndpoint,
			Deployment:   report.Spec.AzureDeployment,
			APIVersion:   report.Spec.AzureAPIVersion,
			APIKey:       apiKey,
			HTTPClient:   r.HTTPClient,
			SystemPrompt: report.Spec.SystemPrompt,
			UserPrompt:   report.Spec.UserPrompt,
		}, nil
	case llm.ProviderOllama:
		if report.Spec.OllamaEndpoint == "" || report.Spec.OllamaModel == "" {
			return nil, fmt.Errorf("ollamaEndpoint and ollamaModel are required when llmProvider is %q", llm.ProviderOllama)
		}
		return llm.Ollama{
			Endpoint:     report.Spec.OllamaEndpoint,
			Model:        report.Spec.OllamaModel,
			APIKey:       os.Getenv("OLLAMA_API_KEY"),
			HTTPClient:   r.HTTPClient,
			SystemPrompt: report.Spec.SystemPrompt,
			UserPrompt:   report.Spec.UserPrompt,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported llmProvider %q", provider)
	}
}

func (r *InsightReportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.InsightReport{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}

func (r *InsightReportReconciler) updateStatus(ctx context.Context, report *v1alpha1.InsightReport, interval time.Duration, reason, message, snapshotHashValue, recommendations string) (ctrl.Result, error) {
	key := client.ObjectKeyFromObject(report)
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var latest v1alpha1.InsightReport
		if err := r.Get(ctx, key, &latest); err != nil {
			return err
		}
		applyStatus(&latest, reason, message, snapshotHashValue, recommendations)
		return r.Status().Update(ctx, &latest)
	}); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: interval}, nil
}

func applyStatus(report *v1alpha1.InsightReport, reason, message, snapshotHashValue, recommendations string) {
	now := metav1.Now()
	report.Status.ObservedAt = &now
	report.Status.Summary = message
	report.Status.LastSnapshot = snapshotHashValue
	report.Status.Recommendations = recommendations
	status := metav1.ConditionTrue
	if reason != "Ready" {
		status = metav1.ConditionFalse
	}
	report.Status.Conditions = []metav1.Condition{{
		Type:               "Ready",
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
		ObservedGeneration: report.Generation,
	}}
}

func parseInterval(value string) time.Duration {
	if value == "" {
		return 30 * time.Minute
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed < time.Minute {
		return 30 * time.Minute
	}
	return parsed
}

func snapshotHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (r *InsightReportReconciler) createSnapshot(ctx context.Context, report *v1alpha1.InsightReport, analyzedAt time.Time, duration time.Duration, recommendations string) error {
	snapshot := &v1alpha1.InsightReportSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: snapshotNamePrefix(report.Name),
			Namespace:    report.Namespace,
		},
		Spec: v1alpha1.InsightReportSnapshotSpec{
			InsightReportName: report.Name,
			InsightReportUID:  string(report.UID),
			AnalyzedAt:        metav1.NewTime(analyzedAt),
			DurationMillis:    duration.Milliseconds(),
			Recommendations:   recommendations,
		},
	}
	if err := controllerutil.SetControllerReference(report, snapshot, r.Scheme); err != nil {
		return err
	}
	return r.Create(ctx, snapshot)
}

func (r *InsightReportReconciler) deleteExpiredSnapshots(ctx context.Context, report *v1alpha1.InsightReport) error {
	var snapshots v1alpha1.InsightReportSnapshotList
	if err := r.List(ctx, &snapshots, client.InNamespace(report.Namespace)); err != nil {
		return err
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays(report.Spec.RetentionDays))
	for i := range snapshots.Items {
		snapshot := &snapshots.Items[i]
		if snapshot.Spec.InsightReportUID != string(report.UID) || !snapshot.Spec.AnalyzedAt.Time.Before(cutoff) {
			continue
		}
		if err := r.Delete(ctx, snapshot); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func retentionDays(value int) int {
	if value < 1 {
		return 30
	}
	return value
}

func snapshotNamePrefix(reportName string) string {
	const maxPrefixLength = 50
	if len(reportName) > maxPrefixLength {
		reportName = reportName[:maxPrefixLength]
	}
	return reportName + "-"
}
