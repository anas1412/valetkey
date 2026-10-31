package admin

import (
	"github.com/anas1412/valetkey/internal/budget"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/anas1412/valetkey/internal/reconciler"
	"github.com/anas1412/valetkey/internal/types"
)

type keyView struct {
	Namespace  string             `json:"namespace"`
	Name       string             `json:"name"`
	Spec       types.ProxyKeySpec `json:"spec"`
	Ready      bool               `json:"ready"`
	Message    string             `json:"message,omitempty"`
	SecretName string             `json:"secretName,omitempty"`
	Requests   int                `json:"requests"`
	// "30d" when Requests comes from Prometheus, "pod" when it's since the pod started.
	RequestsWindow string     `json:"requestsWindow"`
	LastUsed       *int64     `json:"lastUsed,omitempty"`
	Spend          *spendView `json:"spend,omitempty"`
}

func (a *API) viewKey(pk *types.ProxyKey) keyView {
	u := a.Stats.KeyUsage(pk.Namespace, pk.Name)
	return keyView{Namespace: pk.Namespace, Name: pk.Name, Spec: pk.Spec, Ready: pk.Status.Ready, Message: pk.Status.Message,
		SecretName: pk.Status.SecretName, Requests: u.Requests, RequestsWindow: "pod", LastUsed: u.LastUsed,
		Spend: a.spendOf(budget.KeyScope(pk.Namespace, pk.Name), pk.Spec.Budget)}
}

func (a *API) listKeys(w http.ResponseWriter, r *http.Request) {
	var promRequests map[string]int
	if a.Prom != nil {
		promRequests, _ = promKeyRequests(r.Context(), a.Prom) // on error, fall back to memory
	}
	out := []keyView{}
	for _, pk := range a.Controller.ProxyKeys() {
		v := a.viewKey(pk)
		if promRequests != nil {
			v.Requests, v.RequestsWindow = promRequests[pk.Namespace+"/"+pk.Name], "30d"
		}
		out = append(out, v)
	}
	writeJSON(w, 200, out)
}

// findKey answers 404 when the ProxyKey isn't in the informer cache.
func (a *API) findKey(w http.ResponseWriter, r *http.Request) (*types.ProxyKey, bool) {
	pk, ok := a.Controller.ProxyKey(r.PathValue("ns"), r.PathValue("name"))
	if !ok {
		writeMessage(w, 404, "key not found")
	}
	return pk, ok
}

func (a *API) getKey(w http.ResponseWriter, r *http.Request) {
	if pk, ok := a.findKey(w, r); ok {
		writeJSON(w, 200, a.viewKey(pk))
	}
}

func (a *API) createKey(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Namespace string             `json:"namespace"`
		Name      string             `json:"name"`
		Spec      types.ProxyKeySpec `json:"spec"`
	}
	if !readJSON(w, r, &in) || !validName(w, in.Namespace, "tenant") || !validName(w, in.Name, "key name") || !validBudget(w, in.Spec.Budget) {
		return
	}
	pk := types.ProxyKey{
		TypeMeta:   metav1.TypeMeta{APIVersion: types.Group + "/" + types.Version, Kind: "ProxyKey"},
		ObjectMeta: metav1.ObjectMeta{Name: in.Name, Namespace: in.Namespace},
		Spec:       in.Spec,
	}
	if _, err := a.Kube.Create(r.Context(), types.ProxyKeyGVR, in.Namespace, &pk); err != nil {
		writeKubeError(w, err)
		return
	}
	a.audit(r, "created key "+in.Namespace+"/"+in.Name)
	writeJSON(w, 201, keyView{Namespace: in.Namespace, Name: in.Name, Spec: in.Spec})
}

func (a *API) updateKey(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Spec types.ProxyKeySpec `json:"spec"`
	}
	if !readJSON(w, r, &in) || !validBudget(w, in.Spec.Budget) {
		return
	}
	ns, name := r.PathValue("ns"), r.PathValue("name")
	if _, err := a.Kube.ReplaceSpec(r.Context(), types.ProxyKeyGVR, ns, name, in.Spec); err != nil {
		writeKubeError(w, err)
		return
	}
	a.audit(r, "updated key "+ns+"/"+name)
	writeJSON(w, 200, keyView{Namespace: ns, Name: name, Spec: in.Spec})
}

func (a *API) deleteKey(w http.ResponseWriter, r *http.Request) {
	if err := a.Kube.Delete(r.Context(), types.ProxyKeyGVR, r.PathValue("ns"), r.PathValue("name")); err != nil {
		writeKubeError(w, err)
		return
	}
	a.audit(r, "deleted key "+r.PathValue("ns")+"/"+r.PathValue("name"))
	w.WriteHeader(http.StatusNoContent)
}

// revealKey returns the virtual key from the tenant Secret.
func (a *API) revealKey(w http.ResponseWriter, r *http.Request) {
	pk, ok := a.findKey(w, r)
	if !ok {
		return
	}
	key, err := a.Kube.SecretValue(r.Context(), pk.Namespace, reconciler.SecretName(pk.Name), "VALETKEY_KEY")
	if err != nil {
		writeKubeError(w, err)
		return
	}
	if key == "" {
		writeMessage(w, 404, "key not minted yet, check the key status")
		return
	}
	a.audit(r, "revealed key "+pk.Namespace+"/"+pk.Name)
	writeJSON(w, 200, map[string]string{"key": key})
}

// rotateKey deletes the tenant Secret; the reconciler mints a fresh key and the old one stops working.
func (a *API) rotateKey(w http.ResponseWriter, r *http.Request) {
	pk, ok := a.findKey(w, r)
	if !ok {
		return
	}
	if err := a.Kube.DeleteSecret(r.Context(), pk.Namespace, reconciler.SecretName(pk.Name)); err != nil {
		writeKubeError(w, err)
		return
	}
	a.Controller.Enqueue(pk.Namespace, pk.Name)
	a.audit(r, "rotated key "+pk.Namespace+"/"+pk.Name)
	w.WriteHeader(http.StatusNoContent)
}
