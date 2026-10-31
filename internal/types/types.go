// Package types holds the llmproxy CRD shapes and conversions from unstructured objects.
package types

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	Group   = "llmproxy.llmproxy.io"
	Version = "v1alpha1"
)

var (
	ChannelGVR  = schema.GroupVersionResource{Group: Group, Version: Version, Resource: "channels"}
	GroupGVR    = schema.GroupVersionResource{Group: Group, Version: Version, Resource: "groups"}
	ProxyKeyGVR = schema.GroupVersionResource{Group: Group, Version: Version, Resource: "proxykeys"}
	SecretGVR   = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}
	NamespaceGVR = schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}
)

const (
	Enabled  = "Enabled"
	Disabled = "Disabled"
)

// Condition is a standard Kubernetes status condition.
type Condition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason"`
	Message            string `json:"message"`
	LastTransitionTime string `json:"lastTransitionTime"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
}

// Status is the status block shared by all three CRDs (ProxyKey adds two fields).
type Status struct {
	Conditions []Condition `json:"conditions,omitempty"`
	Ready      bool        `json:"ready"`
	Message    string      `json:"message,omitempty"`
	KeyHash    string      `json:"keyHash,omitempty"`
	SecretName string      `json:"secretName,omitempty"`
}

type SecretRef struct {
	Name string `json:"name"`
	Key  string `json:"key,omitempty"`
}

// KeyName is the Secret data key holding the provider API key (default "apiKey").
func (r SecretRef) KeyName() string {
	if r.Key == "" {
		return "apiKey"
	}
	return r.Key
}

type ChannelSpec struct {
	BaseURL       string            `json:"baseURL"`
	Type          string            `json:"type"` // "openai" | "anthropic"
	KeySecretRef  SecretRef         `json:"keySecretRef"`
	Priority      int               `json:"priority,omitempty"`
	Status        string            `json:"status,omitempty"`
	Models        []string          `json:"models,omitempty"`
	ProxyURL      string            `json:"proxyURL,omitempty"`
	SkipTLSVerify bool              `json:"skipTLSVerify,omitempty"`
	ModelMapping  map[string]string `json:"modelMapping,omitempty"`
}

type Channel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ChannelSpec `json:"spec"`
	Status            Status      `json:"status,omitempty"`
}

func (c *Channel) Enabled() bool { return c.Spec.Status == "" || c.Spec.Status == Enabled }

// EffectivePriority defaults to 10.
func (c *Channel) EffectivePriority() int {
	if c.Spec.Priority <= 0 {
		return 10
	}
	return c.Spec.Priority
}

type GroupSpec struct {
	Status      string   `json:"status,omitempty"`
	ChannelRefs []string `json:"channelRefs"`
	RPMRatio    float64  `json:"rpmRatio,omitempty"`
	TPMRatio    float64  `json:"tpmRatio,omitempty"`
}

type TenantGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              GroupSpec `json:"spec"`
	Status            Status    `json:"status,omitempty"`
}

func (g *TenantGroup) Enabled() bool { return g.Spec.Status == "" || g.Spec.Status == Enabled }

type ProxyKeySpec struct {
	Status  string   `json:"status,omitempty"`
	Models  []string `json:"models,omitempty"`
	Subnets []string `json:"subnets,omitempty"`
	RPM     int      `json:"rpm,omitempty"`
	TPM     int      `json:"tpm,omitempty"`
}

type ProxyKey struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ProxyKeySpec `json:"spec"`
	Status            Status       `json:"status,omitempty"`
}

func (k *ProxyKey) Enabled() bool { return k.Spec.Status == "" || k.Spec.Status == Enabled }

// FromUnstructured converts an informer object into a typed CRD.
func FromUnstructured[T any](obj any) (*T, bool) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil, false
	}
	out := new(T)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, out); err != nil {
		return nil, false
	}
	return out, true
}
