package analysis

import "time"

type ClusterSnapshot struct {
	CollectedAt time.Time       `json:"collectedAt"`
	Nodes       []NodeSummary   `json:"nodes"`
	Workloads   []Workload      `json:"workloads"`
	Pods        []PodSummary    `json:"pods"`
	Services    []Service       `json:"services"`
	Events      []EventSummary  `json:"events,omitempty"`
	Metrics     []MetricSummary `json:"metrics,omitempty"`
}

type NodeSummary struct {
	Name              string `json:"name"`
	Ready             bool   `json:"ready"`
	KubeletVersion    string `json:"kubeletVersion"`
	AllocatableCPU    string `json:"allocatableCPU"`
	AllocatableMemory string `json:"allocatableMemory"`
}

type Workload struct {
	Namespace           string `json:"namespace"`
	Kind                string `json:"kind"`
	Name                string `json:"name"`
	DesiredReplicas     int32  `json:"desiredReplicas"`
	AvailableReplicas   int32  `json:"availableReplicas"`
	UnavailableReplicas int32  `json:"unavailableReplicas"`
}

type PodSummary struct {
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Phase     string   `json:"phase"`
	Restarts  int32    `json:"restarts"`
	Ready     bool     `json:"ready"`
	NodeName  string   `json:"nodeName"`
	Warnings  []string `json:"warnings,omitempty"`
}

type Service struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Type      string `json:"type"`
}

type EventSummary struct {
	Namespace string `json:"namespace"`
	Object    string `json:"object"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Count     int32  `json:"count"`
}

type MetricSummary struct {
	Query string `json:"query"`
	Value string `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}
