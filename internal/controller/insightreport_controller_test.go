package controller

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"k8s-insight-controller/api/v1alpha1"
	"k8s-insight-controller/internal/llm"
)

func TestDeleteExpiredSnapshots(t *testing.T) {
	scheme := v1alpha1.NewScheme()
	report := &v1alpha1.InsightReport{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-health", Namespace: "test", UID: types.UID("report-uid")},
		Spec:       v1alpha1.InsightReportSpec{RetentionDays: 7},
	}
	old := snapshotForTest("old", "report-uid", time.Now().AddDate(0, 0, -8))
	recent := snapshotForTest("recent", "report-uid", time.Now().AddDate(0, 0, -2))
	other := snapshotForTest("other", "other-report-uid", time.Now().AddDate(0, 0, -30))

	reconciler := InsightReportReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(report, old, recent, other).Build(),
		Scheme: scheme,
	}
	if err := reconciler.deleteExpiredSnapshots(context.Background(), report); err != nil {
		t.Fatalf("delete expired snapshots: %v", err)
	}

	var snapshots v1alpha1.InsightReportSnapshotList
	if err := reconciler.List(context.Background(), &snapshots, client.InNamespace("test")); err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	if len(snapshots.Items) != 2 {
		t.Fatalf("expected 2 snapshots after retention cleanup, got %d", len(snapshots.Items))
	}
	for _, snapshot := range snapshots.Items {
		if snapshot.Name == "old" {
			t.Fatal("expired snapshot was not deleted")
		}
	}
}

func TestAnalyzerForDefaultsToAzureOpenAI(t *testing.T) {
	t.Setenv("AZURE_OPENAI_API_KEY", "secret")
	reconciler := InsightReportReconciler{}
	report := &v1alpha1.InsightReport{Spec: v1alpha1.InsightReportSpec{
		AzureEndpoint:   "https://example.openai.azure.com",
		AzureDeployment: "deployment",
	}}

	analyzer, err := reconciler.analyzerFor(report)
	if err != nil {
		t.Fatalf("select analyzer: %v", err)
	}
	if _, ok := analyzer.(llm.AzureOpenAI); !ok {
		t.Fatalf("expected AzureOpenAI analyzer, got %T", analyzer)
	}
}

func TestAnalyzerForOllama(t *testing.T) {
	reconciler := InsightReportReconciler{}
	report := &v1alpha1.InsightReport{Spec: v1alpha1.InsightReportSpec{
		LLMProvider:    llm.ProviderOllama,
		OllamaEndpoint: "http://ollama:11434",
		OllamaModel:    "qwen3:8b",
	}}

	analyzer, err := reconciler.analyzerFor(report)
	if err != nil {
		t.Fatalf("select analyzer: %v", err)
	}
	if _, ok := analyzer.(llm.Ollama); !ok {
		t.Fatalf("expected Ollama analyzer, got %T", analyzer)
	}
}

func TestAnalyzerForRejectsUnknownProvider(t *testing.T) {
	reconciler := InsightReportReconciler{}
	report := &v1alpha1.InsightReport{Spec: v1alpha1.InsightReportSpec{LLMProvider: "unknown"}}

	if _, err := reconciler.analyzerFor(report); err == nil {
		t.Fatal("expected unsupported provider error")
	}
}

func snapshotForTest(name, reportUID string, analyzedAt time.Time) *v1alpha1.InsightReportSnapshot {
	return &v1alpha1.InsightReportSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test"},
		Spec: v1alpha1.InsightReportSnapshotSpec{
			InsightReportName: "cluster-health",
			InsightReportUID:  reportUID,
			AnalyzedAt:        metav1.NewTime(analyzedAt),
			Recommendations:   "test",
		},
	}
}
