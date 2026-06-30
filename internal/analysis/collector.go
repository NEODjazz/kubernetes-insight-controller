package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Collector struct {
	Client     client.Client
	HTTPClient *http.Client
}

type CollectOptions struct {
	Namespaces       []string
	PrometheusURL    string
	IncludeEvents    bool
	MaxMetricSamples int
}

func (c *Collector) Collect(ctx context.Context, opts CollectOptions) (*ClusterSnapshot, error) {
	snapshot := &ClusterSnapshot{CollectedAt: time.Now().UTC()}
	namespaces := opts.Namespaces
	if len(namespaces) == 0 {
		namespaces = []string{""}
	}

	if err := c.collectNodes(ctx, snapshot); err != nil {
		return nil, err
	}
	for _, namespace := range namespaces {
		if err := c.collectNamespaced(ctx, namespace, snapshot, opts.IncludeEvents); err != nil {
			return nil, err
		}
	}
	if opts.PrometheusURL != "" {
		snapshot.Metrics = c.collectPrometheus(ctx, opts.PrometheusURL, opts.MaxMetricSamples)
	}
	return snapshot, nil
}

func (c *Collector) collectNodes(ctx context.Context, snapshot *ClusterSnapshot) error {
	var nodes corev1.NodeList
	if err := c.Client.List(ctx, &nodes); err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}
	for _, node := range nodes.Items {
		snapshot.Nodes = append(snapshot.Nodes, NodeSummary{
			Name:              node.Name,
			Ready:             nodeReady(node),
			KubeletVersion:    node.Status.NodeInfo.KubeletVersion,
			AllocatableCPU:    node.Status.Allocatable.Cpu().String(),
			AllocatableMemory: node.Status.Allocatable.Memory().String(),
		})
	}
	return nil
}

func (c *Collector) collectNamespaced(ctx context.Context, namespace string, snapshot *ClusterSnapshot, includeEvents bool) error {
	listOpts := []client.ListOption{}
	if namespace != "" {
		listOpts = append(listOpts, client.InNamespace(namespace))
	}

	var deployments appsv1.DeploymentList
	if err := c.Client.List(ctx, &deployments, listOpts...); err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}
	for _, deployment := range deployments.Items {
		snapshot.Workloads = append(snapshot.Workloads, Workload{
			Namespace:           deployment.Namespace,
			Kind:                "Deployment",
			Name:                deployment.Name,
			DesiredReplicas:     replicasOrZero(deployment.Spec.Replicas),
			AvailableReplicas:   deployment.Status.AvailableReplicas,
			UnavailableReplicas: deployment.Status.UnavailableReplicas,
		})
	}

	var statefulSets appsv1.StatefulSetList
	if err := c.Client.List(ctx, &statefulSets, listOpts...); err != nil {
		return fmt.Errorf("list statefulsets: %w", err)
	}
	for _, sts := range statefulSets.Items {
		snapshot.Workloads = append(snapshot.Workloads, Workload{
			Namespace:         sts.Namespace,
			Kind:              "StatefulSet",
			Name:              sts.Name,
			DesiredReplicas:   replicasOrZero(sts.Spec.Replicas),
			AvailableReplicas: sts.Status.AvailableReplicas,
		})
	}

	var pods corev1.PodList
	if err := c.Client.List(ctx, &pods, listOpts...); err != nil {
		return fmt.Errorf("list pods: %w", err)
	}
	for _, pod := range pods.Items {
		snapshot.Pods = append(snapshot.Pods, summarizePod(pod))
	}

	var services corev1.ServiceList
	if err := c.Client.List(ctx, &services, listOpts...); err != nil {
		return fmt.Errorf("list services: %w", err)
	}
	for _, svc := range services.Items {
		snapshot.Services = append(snapshot.Services, Service{
			Namespace: svc.Namespace,
			Name:      svc.Name,
			Type:      string(svc.Spec.Type),
		})
	}

	if includeEvents {
		var events corev1.EventList
		if err := c.Client.List(ctx, &events, listOpts...); err != nil {
			return fmt.Errorf("list events: %w", err)
		}
		for _, event := range events.Items {
			if event.Type != corev1.EventTypeWarning {
				continue
			}
			snapshot.Events = append(snapshot.Events, EventSummary{
				Namespace: event.Namespace,
				Object:    event.InvolvedObject.Kind + "/" + event.InvolvedObject.Name,
				Reason:    event.Reason,
				Message:   event.Message,
				Count:     event.Count,
			})
		}
	}
	return nil
}

func (c *Collector) collectPrometheus(ctx context.Context, baseURL string, maxSamples int) []MetricSummary {
	queries := []string{
		"sum(rate(container_cpu_usage_seconds_total{container!='',pod!=''}[5m])) by (namespace)",
		"sum(container_memory_working_set_bytes{container!='',pod!=''}) by (namespace)",
		"sum(kube_pod_container_status_restarts_total) by (namespace)",
		"sum(kube_deployment_status_replicas_unavailable) by (namespace, deployment)",
	}
	if maxSamples <= 0 || maxSamples > len(queries) {
		maxSamples = len(queries)
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	var summaries []MetricSummary
	for _, query := range queries[:maxSamples] {
		reqURL, err := prometheusQueryURL(baseURL, query)
		if err != nil {
			summaries = append(summaries, MetricSummary{Query: query, Error: err.Error()})
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			summaries = append(summaries, MetricSummary{Query: query, Error: err.Error()})
			continue
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			summaries = append(summaries, MetricSummary{Query: query, Error: err.Error()})
			continue
		}
		value, err := readPrometheusValue(resp)
		if err != nil {
			summaries = append(summaries, MetricSummary{Query: query, Error: err.Error()})
			continue
		}
		summaries = append(summaries, MetricSummary{Query: query, Value: value})
	}
	return summaries
}

func prometheusQueryURL(baseURL, query string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/") + "/api/v1/query")
	if err != nil {
		return "", err
	}
	params := parsed.Query()
	params.Set("query", query)
	parsed.RawQuery = params.Encode()
	return parsed.String(), nil
}

func readPrometheusValue(resp *http.Response) (string, error) {
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("prometheus returned %s", resp.Status)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	data, _ := payload["data"].(map[string]any)
	result, _ := data["result"].([]any)
	encoded, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func summarizePod(pod corev1.Pod) PodSummary {
	summary := PodSummary{
		Namespace: pod.Namespace,
		Name:      pod.Name,
		Phase:     string(pod.Status.Phase),
		NodeName:  pod.Spec.NodeName,
		Ready:     true,
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status != corev1.ConditionTrue {
			summary.Ready = false
		}
	}
	for _, status := range pod.Status.ContainerStatuses {
		summary.Restarts += status.RestartCount
		if !status.Ready {
			summary.Ready = false
		}
		if status.State.Waiting != nil {
			summary.Warnings = append(summary.Warnings, status.Name+":"+status.State.Waiting.Reason)
		}
	}
	if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodSucceeded {
		summary.Warnings = append(summary.Warnings, string(pod.Status.Phase))
	}
	return summary
}

func replicasOrZero(replicas *int32) int32 {
	if replicas == nil {
		return 0
	}
	return *replicas
}

func nodeReady(node corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func SnapshotJSON(snapshot *ClusterSnapshot) (string, error) {
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func AddKnownTypes(scheme *runtime.Scheme) {
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
}
