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

type channelView struct {
	Name    string            `json:"name"`
	Spec    types.ChannelSpec `json:"spec"`
	Ready   bool              `json:"ready"`
	Message string            `json:"message,omitempty"`
	// KeyRejected: the provider answered 401/403 to the key, so it gets no traffic.
	KeyRejected bool `json:"keyRejected,omitempty"`
}

// channelInput is what the dashboard sends: the spec plus, optionally, the provider API key to store.
type channelInput struct {
	Name   string            `json:"name"`
	Spec   types.ChannelSpec `json:"spec"`
	APIKey string            `json:"apiKey"`
}

// keySecretName is the Secret the dashboard creates for a channel's provider key.
func keySecretName(channel string) string { return channel + "-api-key" }

func viewChannel(e registry.ChannelEntry) channelView {
	msg := e.Channel.Status.Message
	if e.Rejected != "" {
		msg = e.Rejected // this pod's verdict, before (or without) the leader's status write
	}
	return channelView{Name: e.Name(), Spec: e.Channel.Spec, Ready: e.Usable(), Message: msg, KeyRejected: e.Rejected != ""}
}

func (a *API) listChannels(w http.ResponseWriter, _ *http.Request) {
	out := []channelView{}
	for _, e := range a.Registry.Channels() {
		out = append(out, viewChannel(e))
	}
	writeJSON(w, 200, out)
}

func (a *API) getChannel(w http.ResponseWriter, r *http.Request) {
	e, ok := a.Registry.Channel(r.PathValue("name"))
	if !ok {
		writeMessage(w, 404, "channel not found")
		return
	}
	writeJSON(w, 200, viewChannel(e))
}

func (a *API) createChannel(w http.ResponseWriter, r *http.Request) {
	var in channelInput
	if !readJSON(w, r, &in) || !validName(w, in.Name, "channel name") || !validPricing(w, in.Spec.Pricing) {
		return
	}
	if !a.storeKey(w, r, in.Name, &in) {
		return
	}
	ch := types.Channel{
		TypeMeta:   metav1.TypeMeta{APIVersion: types.Group + "/" + types.Version, Kind: "Channel"},
		ObjectMeta: metav1.ObjectMeta{Name: in.Name, Namespace: a.Config.OperatorNamespace},
		Spec:       in.Spec,
	}
	if _, err := a.Kube.Create(r.Context(), types.ChannelGVR, a.Config.OperatorNamespace, &ch); err != nil {
		writeKubeError(w, err)
		return
	}
	a.audit(r, "created provider "+in.Name)
	writeJSON(w, 201, channelView{Name: in.Name, Spec: in.Spec})
}

func (a *API) updateChannel(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var in channelInput
	if !readJSON(w, r, &in) || !validPricing(w, in.Spec.Pricing) {
		return
	}
	if in.Spec.KeySecretRef.Name == "" || in.Spec.Pricing == nil {
		// Keep the live object's key reference and price overrides (set in YAML)
		// when the dashboard doesn't send them.
		cur, err := a.Kube.Get(r.Context(), types.ChannelGVR, a.Config.OperatorNamespace, name)
		if err != nil {
			writeKubeError(w, err)
			return
		}
		if ch, ok := types.FromUnstructured[types.Channel](cur); ok {
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
	if _, err := a.Kube.ReplaceSpec(r.Context(), types.ChannelGVR, a.Config.OperatorNamespace, name, in.Spec); err != nil {
		writeKubeError(w, err)
		return
	}
	if in.APIKey != "" {
		a.audit(r, "updated provider "+name+" and replaced its API key")
	} else {
		a.audit(r, "updated provider "+name)
	}
	writeJSON(w, 200, channelView{Name: name, Spec: in.Spec})
}

// storeKey saves a pasted API key in a managed Secret and points the spec at it.
func (a *API) storeKey(w http.ResponseWriter, r *http.Request, channel string, in *channelInput) bool {
	if in.APIKey != "" {
		in.Spec.KeySecretRef = types.SecretRef{Name: keySecretName(channel), Key: "apiKey"}
		err := a.Kube.ApplySecret(r.Context(), a.Config.OperatorNamespace, keySecretName(channel),
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

func (a *API) deleteChannel(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	e, known := a.Registry.Channel(name)
	ns := a.Config.OperatorNamespace
	if err := a.Kube.Delete(r.Context(), types.ChannelGVR, ns, name); err != nil {
		writeKubeError(w, err)
		return
	}
	// Only remove the key Secret if the dashboard created it.
	if known && e.Channel.Spec.KeySecretRef.Name == keySecretName(name) {
		if s, err := a.Kube.ReadSecret(r.Context(), ns, keySecretName(name)); err == nil && s != nil && s.Labels["app.kubernetes.io/managed-by"] == kube.ManagedBy {
			_ = a.Kube.DeleteSecret(r.Context(), ns, s.Name)
		}
	}
	a.audit(r, "deleted provider "+name)
	w.WriteHeader(http.StatusNoContent)
}

// testChannel checks that the provider accepts the channel's key by listing its models.
func (a *API) testChannel(w http.ResponseWriter, r *http.Request) {
	e, ok := a.Registry.Channel(r.PathValue("name"))
	if !ok {
		writeMessage(w, 404, "channel not found")
		return
	}
	if e.UpstreamKey == "" {
		writeJSON(w, 200, map[string]any{"success": false, "latencyMs": 0, "message": "no API key resolved from the channel Secret"})
		return
	}
	res, err := fetchModels(r.Context(), e.Channel, e.UpstreamKey)
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

func validPricing(w http.ResponseWriter, p *types.ChannelPricing) bool {
	if err := pricing.ValidateOverrides(p); err != nil {
		writeMessage(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}
