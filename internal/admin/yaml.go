package admin

import (
	"net/http"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	"github.com/anas1412/valetkey/internal/types"
)

func (a *API) channelYAML(w http.ResponseWriter, r *http.Request) {
	a.writeYAML(w, r, types.ChannelGVR, a.Config.OperatorNamespace, r.PathValue("name"))
}

func (a *API) groupYAML(w http.ResponseWriter, r *http.Request) {
	a.writeYAML(w, r, types.GroupGVR, a.Config.OperatorNamespace, r.PathValue("name"))
}

func (a *API) keyYAML(w http.ResponseWriter, r *http.Request) {
	a.writeYAML(w, r, types.ProxyKeyGVR, r.PathValue("ns"), r.PathValue("name"))
}

// writeYAML answers with the object as it is in the cluster, reduced to what you
// would commit to Git: no status and no fields the API server manages.
func (a *API) writeYAML(w http.ResponseWriter, r *http.Request, gvr schema.GroupVersionResource, ns, name string) {
	obj, err := a.Kube.Get(r.Context(), gvr, ns, name)
	if err != nil {
		writeKubeError(w, err)
		return
	}
	meta := map[string]any{"name": obj.GetName(), "namespace": obj.GetNamespace()}
	if l := obj.GetLabels(); len(l) > 0 {
		meta["labels"] = l
	}
	ann := obj.GetAnnotations()
	delete(ann, "kubectl.kubernetes.io/last-applied-configuration")
	if len(ann) > 0 {
		meta["annotations"] = ann
	}
	clean := map[string]any{"apiVersion": obj.GetAPIVersion(), "kind": obj.GetKind(), "metadata": meta, "spec": obj.Object["spec"]}
	out, err := yaml.Marshal(clean)
	if err != nil {
		writeMessage(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(out)
}
