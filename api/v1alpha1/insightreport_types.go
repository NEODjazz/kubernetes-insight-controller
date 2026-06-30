package v1alpha1

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type InsightReportSpec struct {
	Namespaces       []string `json:"namespaces,omitempty"`
	PrometheusURL    string   `json:"prometheusURL,omitempty"`
	Interval         string   `json:"interval,omitempty"`
	LLMProvider      string   `json:"llmProvider,omitempty"`
	AzureEndpoint    string   `json:"azureEndpoint,omitempty"`
	AzureDeployment  string   `json:"azureDeployment,omitempty"`
	AzureAPIVersion  string   `json:"azureAPIVersion,omitempty"`
	OllamaEndpoint   string   `json:"ollamaEndpoint,omitempty"`
	OllamaModel      string   `json:"ollamaModel,omitempty"`
	MaxMetricSamples int      `json:"maxMetricSamples,omitempty"`
	IncludeEvents    bool     `json:"includeEvents,omitempty"`
	RetentionDays    int      `json:"retentionDays,omitempty"`
	SystemPrompt     string   `json:"systemPrompt,omitempty"`
	UserPrompt       string   `json:"userPrompt,omitempty"`
}

type InsightReportStatus struct {
	ObservedAt      *v1.Time       `json:"observedAt,omitempty"`
	Summary         string         `json:"summary,omitempty"`
	Recommendations string         `json:"recommendations,omitempty"`
	LastSnapshot    string         `json:"lastSnapshot,omitempty"`
	Conditions      []v1.Condition `json:"conditions,omitempty"`
}

type InsightReport struct {
	v1.TypeMeta   `json:",inline"`
	v1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InsightReportSpec   `json:"spec,omitempty"`
	Status InsightReportStatus `json:"status,omitempty"`
}

type InsightReportList struct {
	v1.TypeMeta `json:",inline"`
	v1.ListMeta `json:"metadata,omitempty"`
	Items       []InsightReport `json:"items"`
}

func (in *InsightReport) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(InsightReport)
	*out = *in
	out.ObjectMeta = *in.ObjectMeta.DeepCopy()
	if in.Spec.Namespaces != nil {
		out.Spec.Namespaces = append([]string{}, in.Spec.Namespaces...)
	}
	if in.Status.Conditions != nil {
		out.Status.Conditions = append([]v1.Condition{}, in.Status.Conditions...)
	}
	if in.Status.ObservedAt != nil {
		out.Status.ObservedAt = in.Status.ObservedAt.DeepCopy()
	}
	return out
}

func (in *InsightReportList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(InsightReportList)
	*out = *in
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		out.Items = make([]InsightReport, len(in.Items))
		for i := range in.Items {
			out.Items[i] = *in.Items[i].DeepCopyObject().(*InsightReport)
		}
	}
	return out
}

type InsightReportSnapshotSpec struct {
	InsightReportName string  `json:"insightReportName"`
	InsightReportUID  string  `json:"insightReportUID"`
	AnalyzedAt        v1.Time `json:"analyzedAt"`
	DurationMillis    int64   `json:"durationMillis"`
	Recommendations   string  `json:"recommendations"`
}

type InsightReportSnapshot struct {
	v1.TypeMeta   `json:",inline"`
	v1.ObjectMeta `json:"metadata,omitempty"`

	Spec InsightReportSnapshotSpec `json:"spec,omitempty"`
}

type InsightReportSnapshotList struct {
	v1.TypeMeta `json:",inline"`
	v1.ListMeta `json:"metadata,omitempty"`
	Items       []InsightReportSnapshot `json:"items"`
}

func (in *InsightReportSnapshot) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(InsightReportSnapshot)
	*out = *in
	out.ObjectMeta = *in.ObjectMeta.DeepCopy()
	out.Spec.AnalyzedAt = *in.Spec.AnalyzedAt.DeepCopy()
	return out
}

func (in *InsightReportSnapshotList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(InsightReportSnapshotList)
	*out = *in
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		out.Items = make([]InsightReportSnapshot, len(in.Items))
		for i := range in.Items {
			out.Items[i] = *in.Items[i].DeepCopyObject().(*InsightReportSnapshot)
		}
	}
	return out
}
