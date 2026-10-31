package admin

import (
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/anas1412/valetkey/internal/types"
)

type groupView struct {
	Name    string          `json:"name"`
	Spec    types.GroupSpec `json:"spec"`
	Ready   bool            `json:"ready"`
	Message string          `json:"message,omitempty"`
	Keys    int             `json:"keys"`
}

func (a *API) viewGroup(g *types.TenantGroup) groupView {
	n := 0
	for _, pk := range a.Controller.ProxyKeys() {
		if pk.Namespace == g.Name {
			n++
		}
	}
	return groupView{Name: g.Name, Spec: g.Spec, Ready: g.Status.Ready, Message: g.Status.Message, Keys: n}
}

func (a *API) listGroups(w http.ResponseWriter, _ *http.Request) {
	out := []groupView{}
	for _, g := range a.Registry.Groups() {
		out = append(out, a.viewGroup(g))
	}
	writeJSON(w, 200, out)
}

func (a *API) getGroup(w http.ResponseWriter, r *http.Request) {
	g, ok := a.Registry.Group(r.PathValue("name"))
	if !ok {
		writeMessage(w, 404, "group not found")
		return
	}
	writeJSON(w, 200, a.viewGroup(g))
}

func (a *API) createGroup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string          `json:"name"`
		Spec types.GroupSpec `json:"spec"`
	}
	if !readJSON(w, r, &in) || !validName(w, in.Name, "tenant name") {
		return
	}
	g := types.TenantGroup{
		TypeMeta:   metav1.TypeMeta{APIVersion: types.Group + "/" + types.Version, Kind: "Group"},
		ObjectMeta: metav1.ObjectMeta{Name: in.Name, Namespace: a.Config.OperatorNamespace},
		Spec:       in.Spec,
	}
	if _, err := a.Kube.Create(r.Context(), types.GroupGVR, a.Config.OperatorNamespace, &g); err != nil {
		writeKubeError(w, err)
		return
	}
	a.audit(r, "created tenant "+in.Name)
	writeJSON(w, 201, groupView{Name: in.Name, Spec: in.Spec})
}

func (a *API) updateGroup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Spec types.GroupSpec `json:"spec"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	name := r.PathValue("name")
	if _, err := a.Kube.ReplaceSpec(r.Context(), types.GroupGVR, a.Config.OperatorNamespace, name, in.Spec); err != nil {
		writeKubeError(w, err)
		return
	}
	a.audit(r, "updated tenant "+name)
	writeJSON(w, 200, groupView{Name: name, Spec: in.Spec})
}

func (a *API) deleteGroup(w http.ResponseWriter, r *http.Request) {
	if err := a.Kube.Delete(r.Context(), types.GroupGVR, a.Config.OperatorNamespace, r.PathValue("name")); err != nil {
		writeKubeError(w, err)
		return
	}
	a.audit(r, "deleted tenant "+r.PathValue("name"))
	w.WriteHeader(http.StatusNoContent)
}
