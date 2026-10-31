package admin

import (
	"bytes"
	"fmt"
	"net/http"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	"github.com/anas1412/valetkey/internal/types"
)

// channelYAML answers with the Channel. Its key Secret is never shown: the value
// must stay out of the dashboard, so a comment says where it goes.
func (a *API) channelYAML(w http.ResponseWriter, r *http.Request) {
	obj, err := a.Kube.Get(r.Context(), types.ChannelGVR, a.Config.OperatorNamespace, r.PathValue("name"))
	if err != nil {
		writeKubeError(w, err)
		return
	}
	secret, _, _ := unstructured.NestedString(obj.Object, "spec", "keySecretRef", "name")
	note := fmt.Sprintf("# The API key Secret %q is not shown. In Git, put it here, encrypted, followed by ---\n", secret)
	writeYAMLDocs(w, note, forGit(obj))
}

func (a *API) keyYAML(w http.ResponseWriter, r *http.Request) {
	a.writeYAML(w, r, types.ProxyKeyGVR, r.PathValue("ns"), r.PathValue("name"))
}

// groupYAML answers with the tenant's whole file, as laid out in the GitOps guide:
// its Namespace, its Group and every ProxyKey in that namespace.
func (a *API) groupYAML(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	g, err := a.Kube.Get(r.Context(), types.GroupGVR, a.Config.OperatorNamespace, name)
	if err != nil {
		writeKubeError(w, err)
		return
	}
	keys, err := a.Kube.Dynamic.Resource(types.ProxyKeyGVR).Namespace(name).List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeKubeError(w, err)
		return
	}
	sort.Slice(keys.Items, func(i, j int) bool { return keys.Items[i].GetName() < keys.Items[j].GetName() })

	docs := []any{
		map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": name}},
		forGit(g),
	}
	for i := range keys.Items {
		docs = append(docs, forGit(&keys.Items[i]))
	}
	writeYAMLDocs(w, "", docs...)
}

// writeYAML answers with one object as it is in the cluster, reduced to what you
// would commit to Git.
func (a *API) writeYAML(w http.ResponseWriter, r *http.Request, gvr schema.GroupVersionResource, ns, name string) {
	obj, err := a.Kube.Get(r.Context(), gvr, ns, name)
	if err != nil {
		writeKubeError(w, err)
		return
	}
	writeYAMLDocs(w, "", forGit(obj))
}

// forGit keeps what you would commit: no status and no fields the API server manages.
func forGit(obj *unstructured.Unstructured) map[string]any {
	meta := map[string]any{"name": obj.GetName(), "namespace": obj.GetNamespace()}
	if l := obj.GetLabels(); len(l) > 0 {
		meta["labels"] = l
	}
	ann := obj.GetAnnotations()
	delete(ann, "kubectl.kubernetes.io/last-applied-configuration")
	if len(ann) > 0 {
		meta["annotations"] = ann
	}
	return map[string]any{"apiVersion": obj.GetAPIVersion(), "kind": obj.GetKind(), "metadata": meta, "spec": obj.Object["spec"]}
}

// writeYAMLDocs writes the objects as one YAML file, separated by "---", after an
// optional comment.
func writeYAMLDocs(w http.ResponseWriter, comment string, docs ...any) {
	buf := bytes.NewBufferString(comment)
	for i, d := range docs {
		out, err := yaml.Marshal(d)
		if err != nil {
			writeMessage(w, 500, err.Error())
			return
		}
		if i > 0 {
			buf.WriteString("---\n")
		}
		buf.Write(out)
	}
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(buf.Bytes())
}
