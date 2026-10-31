// Package types holds the valetkey CRD shapes and conversions from unstructured objects.
package types

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	APIGroup = "valetkey.dev"
	Version  = "v1alpha1"
)

var (
	ProviderGVR  = schema.GroupVersionResource{Group: APIGroup, Version: Version, Resource: "providers"}
	TenantGVR    = schema.GroupVersionResource{Group: APIGroup, Version: Version, Resource: "tenants"}
	ProxyKeyGVR  = schema.GroupVersionResource{Group: APIGroup, Version: Version, Resource: "proxykeys"}
	SecretGVR    = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}
	ConfigMapGVR = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	EventGVR     = schema.GroupVersionResource{Version: "v1", Resource: "events"}
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
// RotateAnnotation on a ProxyKey: setting it to a new value issues a new key, so
// keys can be rotated from Git.
const RotateAnnotation = "valetkey.dev/rotate"

type Status struct {
	Conditions []Condition `json:"conditions,omitempty"`
	Ready      bool        `json:"ready"`
	Message    string      `json:"message,omitempty"`
	KeyHash    string      `json:"keyHash,omitempty"`
	SecretName string      `json:"secretName,omitempty"`
	// Rotation is the RotateAnnotation value the current key was issued for (ProxyKey).
	Rotation string `json:"rotation,omitempty"`
	// Spend is the current period's spend (ProxyKey and Tenant).
	Spend *Spend `json:"spend,omitempty"`
	// ModelSpend is the spend per model with a budget in spec.modelBudgets (ProxyKey).
	ModelSpend map[string]*Spend `json:"modelSpend,omitempty"`
}

// Spend is kept in status so budgets survive restarts and are shared between
// replicas: each pod writes its own share under Pods, and USD is the total.
// Amounts are exact decimals.
type Spend struct {
	PeriodStart string               `json:"periodStart"`
	USD         string               `json:"usd"`
	Requests    int64                `json:"requests"`
	Unpriced    int64                `json:"unpriced,omitempty"`
	UpdatedAt   string               `json:"updatedAt"`
	Pods        map[string]*PodSpend `json:"pods,omitempty"` // nil entry = remove (merge patch)
}

// PodSpend is one replica's share of a period's spend.
type PodSpend struct {
	PeriodStart string `json:"periodStart"`
	USD         string `json:"usd"`
	Requests    int64  `json:"requests"`
	Unpriced    int64  `json:"unpriced,omitempty"`
}

// Budget caps spend per period. USD is a decimal string ("20", "0.50").
type Budget struct {
	USD      string `json:"usd"`
	Period   string `json:"period,omitempty"`   // daily | weekly | monthly (default)
	Timezone string `json:"timezone,omitempty"` // IANA name; default UTC
}

// ProviderPricing says how a provider's requests are priced.
type ProviderPricing struct {
	// Provider is the price catalogue provider (models.dev id, e.g. "deepseek").
	// Empty: guessed from baseURL.
	Provider string `json:"provider,omitempty"`
	// Models are per-model prices that win over the catalogue (negotiated rates,
	// self-hosted models, Azure deployment names).
	Models map[string]ModelPrice `json:"models,omitempty"`
}

// ModelPrice is in dollars per million tokens, as decimal strings.
type ModelPrice struct {
	Input        string `json:"input"`
	Output       string `json:"output"`
	CacheRead    string `json:"cacheRead,omitempty"`
	CacheWrite   string `json:"cacheWrite,omitempty"`
	CacheWrite1h string `json:"cacheWrite1h,omitempty"`
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

type ProviderSpec struct {
	BaseURL       string            `json:"baseURL"`
	Type          string            `json:"type"` // "openai" | "anthropic"
	KeySecretRef  SecretRef         `json:"keySecretRef"`
	Priority      int               `json:"priority,omitempty"`
	Status        string            `json:"status,omitempty"`
	Models        []string          `json:"models,omitempty"`
	ProxyURL      string            `json:"proxyURL,omitempty"`
	SkipTLSVerify bool              `json:"skipTLSVerify,omitempty"`
	ModelMapping  map[string]string `json:"modelMapping,omitempty"`
	Pricing       *ProviderPricing  `json:"pricing,omitempty"`
}

type Provider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ProviderSpec `json:"spec"`
	Status            Status       `json:"status,omitempty"`
}

func (c *Provider) Enabled() bool { return c.Spec.Status == "" || c.Spec.Status == Enabled }

// EffectivePriority defaults to 10.
func (c *Provider) EffectivePriority() int {
	if c.Spec.Priority <= 0 {
		return 10
	}
	return c.Spec.Priority
}

type TenantSpec struct {
	Status    string   `json:"status,omitempty"`
	Providers []string `json:"providers"`
	RPMRatio  float64  `json:"rpmRatio,omitempty"`
	TPMRatio  float64  `json:"tpmRatio,omitempty"`
	Budget    *Budget  `json:"budget,omitempty"`
}

type Tenant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              TenantSpec `json:"spec"`
	Status            Status     `json:"status,omitempty"`
}

func (g *Tenant) Enabled() bool { return g.Spec.Status == "" || g.Spec.Status == Enabled }

type ProxyKeySpec struct {
	Status  string   `json:"status,omitempty"`
	Models  []string `json:"models,omitempty"`
	Subnets []string `json:"subnets,omitempty"`
	RPM     int      `json:"rpm,omitempty"`
	TPM     int      `json:"tpm,omitempty"`
	Budget  *Budget  `json:"budget,omitempty"`
	// MaxTokens is the output cap added to requests that don't set one.
	MaxTokens int `json:"maxTokens,omitempty"`
	// ExpiresAt (RFC 3339) is when the key stops working.
	ExpiresAt string `json:"expiresAt,omitempty"`
	// ModelBudgets caps spend on single models, e.g. {"claude-opus-4-5": {usd: "5"}}.
	ModelBudgets map[string]Budget `json:"modelBudgets,omitempty"`
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
