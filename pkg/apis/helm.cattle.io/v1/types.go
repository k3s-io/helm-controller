package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/util/intstr"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type HelmChart struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HelmChartSpec   `json:"spec,omitempty"`
	Status HelmChartStatus `json:"status,omitempty"`
}

type HelmChartSpec struct {
	TargetNamespace string                        `json:"targetNamespace,omitempty"`
	CreateNamespace bool                          `json:"createNamespace,omitempty"`
	Chart           string                        `json:"chart,omitempty"`
	Version         string                        `json:"version,omitempty"`
	Repo            string                        `json:"repo,omitempty"`
	RepoCA          string                        `json:"repoCA,omitempty"`
	RepoCAConfigMap *corev1.LocalObjectReference  `json:"repoCAConfigMap,omitempty"`
	Set             map[string]intstr.IntOrString `json:"set,omitempty"`
	ValuesContent   string                        `json:"valuesContent,omitempty"`
	HelmVersion     string                        `json:"helmVersion,omitempty"`
	Bootstrap       bool                          `json:"bootstrap,omitempty"`
	ChartContent    string                        `json:"chartContent,omitempty"`
	JobImage        string                        `json:"jobImage,omitempty"`
	BackOffLimit    *int32                        `json:"backOffLimit,omitempty"`
	Timeout         *metav1.Duration              `json:"timeout,omitempty"`
	FailurePolicy   string                        `json:"failurePolicy,omitempty"`
	AuthSecret      *corev1.LocalObjectReference  `json:"authSecret,omitempty"`

	AuthPassCredentials   bool `json:"authPassCredentials,omitempty"`
	InsecureSkipTLSVerify bool `json:"insecureSkipTLSVerify,omitempty"`
	PlainHTTP             bool `json:"plainHTTP,omitempty"`

	DockerRegistrySecret *corev1.LocalObjectReference `json:"dockerRegistrySecret,omitempty"`
	PodSecurityContext   *corev1.PodSecurityContext   `json:"podSecurityContext,omitempty"`
	SecurityContext      *corev1.SecurityContext      `json:"securityContext,omitempty"`
}

type HelmChartStatus struct {
	JobName    string               `json:"jobName,omitempty"`
	Conditions []HelmChartCondition `json:"conditions,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type HelmChartConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec HelmChartConfigSpec `json:"spec,omitempty"`
}

type HelmChartConfigSpec struct {
	ValuesContent string `json:"valuesContent,omitempty"`
	FailurePolicy string `json:"failurePolicy,omitempty"`
}

type HelmChartConditionType string

const (
	HelmChartJobCreated HelmChartConditionType = "JobCreated"
	HelmChartFailed     HelmChartConditionType = "Failed"
)

type HelmChartCondition struct {
	// Type of job condition.
	Type HelmChartConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// (brief) reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Human readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}
