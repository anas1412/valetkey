package admin

import (
	"github.com/anas1412/valetkey/internal/budget"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/anas1412/valetkey/internal/types"
)

type tenantView struct {
	Name    string           `json:"name"`
	Spec    types.TenantSpec `json:"spec"`
	Ready   bool             `json:"ready"`
	Message string           `json:"message,omitempty"`
	Keys    int              `json:"keys"`
	Spend   *spendView       `json:"spend,omitempty"`
}

func (a *API) viewTenant(g *types.Tenant) tenantView {
	n := 0
	for _, pk := range a.Controller.ProxyKeys() {
		if pk.Namespace == g.Name {
			n++
		}
	}
	return tenantView{Name: g.Name, Spec: g.Spec, Ready: g.Status.Ready, Message: g.Status.Message, Keys: n,
		Spend: a.spendOf(budget.TenantScope(g.Name), g.Spec.Budget)}
}

func (a *API) listTenants(w http.ResponseWriter, _ *http.Request) {
	out := []tenantView{}
	for _, g := range a.Registry.Tenants() {
		out = append(out, a.viewTenant(g))
	}
	writeJSON(w, 200, out)
}

func (a *API) getTenant(w http.ResponseWriter, r *http.Request) {
	g, ok := a.Registry.Tenant(r.PathValue("name"))
	if !ok {
		writeMessage(w, 404, "tenant not found")
		return
	}
	writeJSON(w, 200, a.viewTenant(g))
}

func (a *API) createTenant(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string           `json:"name"`
		Spec types.TenantSpec `json:"spec"`
	}
	if !readJSON(w, r, &in) || !validName(w, in.Name, "tenant name") || !validBudget(w, in.Spec.Budget) {
		return
	}
	g := types.Tenant{
		TypeMeta:   metav1.TypeMeta{APIVersion: types.APIGroup + "/" + types.Version, Kind: "Tenant"},
		ObjectMeta: metav1.ObjectMeta{Name: in.Name, Namespace: a.Config.OperatorNamespace},
		Spec:       in.Spec,
	}
	if _, err := a.Kube.Create(r.Context(), types.TenantGVR, a.Config.OperatorNamespace, &g); err != nil {
		writeKubeError(w, err)
		return
	}
	a.audit(r, "created tenant "+in.Name)
	writeJSON(w, 201, tenantView{Name: in.Name, Spec: in.Spec})
}

func (a *API) updateTenant(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Spec types.TenantSpec `json:"spec"`
	}
	if !readJSON(w, r, &in) || !validBudget(w, in.Spec.Budget) {
		return
	}
	name := r.PathValue("name")
	if _, err := a.Kube.ReplaceSpec(r.Context(), types.TenantGVR, a.Config.OperatorNamespace, name, in.Spec); err != nil {
		writeKubeError(w, err)
		return
	}
	a.audit(r, "updated tenant "+name)
	writeJSON(w, 200, tenantView{Name: name, Spec: in.Spec})
}

func (a *API) deleteTenant(w http.ResponseWriter, r *http.Request) {
	if err := a.Kube.Delete(r.Context(), types.TenantGVR, a.Config.OperatorNamespace, r.PathValue("name")); err != nil {
		writeKubeError(w, err)
		return
	}
	a.audit(r, "deleted tenant "+r.PathValue("name"))
	w.WriteHeader(http.StatusNoContent)
}
