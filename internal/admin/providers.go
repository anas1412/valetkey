package admin

import (
	"fmt"
	"github.com/anas1412/valetkey/internal/pricing"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/anas1412/valetkey/internal/kube"
	"github.com/anas1412/valetkey/internal/registry"
	"github.com/anas1412/valetkey/internal/relay"
	"github.com/anas1412/valetkey/internal/types"
)

type providerView struct {
	Name    string             `json:"name"`
	Spec    types.ProviderSpec `json:"spec"`
	Ready   bool               `json:"ready"`
	Message string             `json:"message,omitempty"`
	// KeyRejected: the provider answered 401/403 to the key, so it gets no traffic.
	KeyRejected bool `json:"keyRejected,omitempty"`
}

// providerInput is what the dashboard sends: the spec plus, optionally, the provider API key to store.
type providerInput struct {
	Name   string             `json:"name"`
	Spec   types.ProviderSpec `json:"spec"`
	APIKey string             `json:"apiKey"`
}

// keySecretName is the Secret the dashboard creates for a provider's provider key.
func keySecretName(provider string) string { return provider + "-api-key" }

func viewProvider(e registry.ProviderEntry) providerView {
	msg := e.Provider.Status.Message
	if e.Rejected != "" {
		msg = e.Rejected // this pod's verdict, before (or without) the leader's status write
	}
	return providerView{Name: e.Name(), Spec: e.Provider.Spec, Ready: e.Usable(), Message: msg, KeyRejected: e.Rejected != ""}
}

func (a *API) listProviders(w http.ResponseWriter, _ *http.Request) {
	out := []providerView{}
	for _, e := range a.Registry.Providers() {
		out = append(out, viewProvider(e))
	}
	writeJSON(w, 200, out)
}

func (a *API) getProvider(w http.ResponseWriter, r *http.Request) {
	e, ok := a.Registry.Provider(r.PathValue("name"))
	if !ok {
		writeMessage(w, 404, "provider not found")
		return
	}
	writeJSON(w, 200, viewProvider(e))
}

func (a *API) createProvider(w http.ResponseWriter, r *http.Request) {
	var in providerInput
	if !readJSON(w, r, &in) || !validName(w, in.Name, "provider name") || !validPricing(w, in.Spec.Pricing) {
		return
	}
	if !a.storeKey(w, r, in.Name, &in) {
		return
	}
	ch := types.Provider{
		TypeMeta:   metav1.TypeMeta{APIVersion: types.APIGroup + "/" + types.Version, Kind: "Provider"},
		ObjectMeta: metav1.ObjectMeta{Name: in.Name, Namespace: a.Config.OperatorNamespace},
		Spec:       in.Spec,
	}
	if _, err := a.Kube.Create(r.Context(), types.ProviderGVR, a.Config.OperatorNamespace, &ch); err != nil {
		writeKubeError(w, err)
		return
	}
	a.audit(r, "created provider "+in.Name)
	writeJSON(w, 201, providerView{Name: in.Name, Spec: in.Spec})
}

func (a *API) updateProvider(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var in providerInput
	if !readJSON(w, r, &in) || !validPricing(w, in.Spec.Pricing) {
		return
	}
	if in.Spec.KeySecretRef.Name == "" || in.Spec.Pricing == nil {
		// Keep the live object's key reference and price overrides (set in YAML)
		// when the dashboard doesn't send them.
		cur, err := a.Kube.Get(r.Context(), types.ProviderGVR, a.Config.OperatorNamespace, name)
		if err != nil {
			writeKubeError(w, err)
			return
		}
		if ch, ok := types.FromUnstructured[types.Provider](cur); ok {
			if in.Spec.KeySecretRef.Name == "" {
				in.Spec.KeySecretRef = ch.Spec.KeySecretRef
			}
			if in.Spec.Pricing == nil {
				in.Spec.Pricing = ch.Spec.Pricing
			}
		}
	}
	if !a.storeKey(w, r, name, &in) {
		return
	}
	if _, err := a.Kube.ReplaceSpec(r.Context(), types.ProviderGVR, a.Config.OperatorNamespace, name, in.Spec); err != nil {
		writeKubeError(w, err)
		return
	}
	if in.APIKey != "" {
		a.audit(r, "updated provider "+name+" and replaced its API key")
	} else {
		a.audit(r, "updated provider "+name)
	}
	writeJSON(w, 200, providerView{Name: name, Spec: in.Spec})
}

// storeKey saves a pasted API key in a managed Secret and points the spec at it.
func (a *API) storeKey(w http.ResponseWriter, r *http.Request, provider string, in *providerInput) bool {
	if in.APIKey != "" {
		in.Spec.KeySecretRef = types.SecretRef{Name: keySecretName(provider), Key: "apiKey"}
		err := a.Kube.ApplySecret(r.Context(), a.Config.OperatorNamespace, keySecretName(provider),
			map[string]string{"app.kubernetes.io/managed-by": kube.ManagedBy}, nil, map[string]string{"apiKey": in.APIKey})
		if err != nil {
			writeKubeError(w, err)
			return false
		}
	}
	if in.Spec.KeySecretRef.Name == "" {
		writeMessage(w, 400, "an API key is required")
		return false
	}
	return true
}

func (a *API) deleteProvider(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	e, known := a.Registry.Provider(name)
	ns := a.Config.OperatorNamespace
	if err := a.Kube.Delete(r.Context(), types.ProviderGVR, ns, name); err != nil {
		writeKubeError(w, err)
		return
	}
	// Only remove the key Secret if the dashboard created it.
	if known && e.Provider.Spec.KeySecretRef.Name == keySecretName(name) {
		if s, err := a.Kube.ReadSecret(r.Context(), ns, keySecretName(name)); err == nil && s != nil && s.Labels["app.kubernetes.io/managed-by"] == kube.ManagedBy {
			_ = a.Kube.DeleteSecret(r.Context(), ns, s.Name)
		}
	}
	a.audit(r, "deleted provider "+name)
	w.WriteHeader(http.StatusNoContent)
}

// testProvider checks that the provider accepts the provider's key by listing its models.
func (a *API) testProvider(w http.ResponseWriter, r *http.Request) {
	e, ok := a.Registry.Provider(r.PathValue("name"))
	if !ok {
		writeMessage(w, 404, "provider not found")
		return
	}
	if e.UpstreamKey == "" {
		writeJSON(w, 200, map[string]any{"success": false, "latencyMs": 0, "message": "no API key resolved from the provider Secret"})
		return
	}
	res, err := fetchModels(r.Context(), e.Provider, e.UpstreamKey)
	if a.Controller != nil {
		a.Controller.KeyVerdict(e.Name(), relay.RejectedReason(err))
	}
	out := map[string]any{"success": err == nil, "latencyMs": res.LatencyMs, "status": res.Status}
	if err != nil {
		out["message"] = err.Error()
	} else {
		out["message"] = fmt.Sprintf("reachable, key accepted, %d models", len(res.Models))
	}
	writeJSON(w, 200, out)
}

func validPricing(w http.ResponseWriter, p *types.ProviderPricing) bool {
	if err := pricing.ValidateOverrides(p); err != nil {
		writeMessage(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}
