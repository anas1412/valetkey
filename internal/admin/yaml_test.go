package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynfake "k8s.io/client-go/dynamic/fake"

	"github.com/anas1412/valetkey/internal/config"
	"github.com/anas1412/valetkey/internal/kube"
	"github.com/anas1412/valetkey/internal/registry"
	"github.com/anas1412/valetkey/internal/stats"
	"github.com/anas1412/valetkey/internal/types"
)

// View YAML returns what you would commit: spec and labels, no status or server fields.
func TestKeyYAML(t *testing.T) {
	key := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "valetkey.dev/v1alpha1", "kind": "ProxyKey",
		"metadata": map[string]any{
			"name": "invoice-api", "namespace": "billing", "uid": "abc", "resourceVersion": "7",
			"managedFields": []any{map[string]any{"manager": "kubectl"}},
			"labels":        map[string]any{"app": "invoices"},
			"annotations":   map[string]any{"kubectl.kubernetes.io/last-applied-configuration": "{}"},
		},
		"spec":   map[string]any{"rpm": int64(60), "models": []any{"gpt-4o-mini"}},
		"status": map[string]any{"ready": true},
	}}
	dyn := dynfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{types.ProxyKeyGVR: "ProxyKeyList"})
	if _, err := dyn.Resource(types.ProxyKeyGVR).Namespace("billing").Create(t.Context(), key, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	api := &API{Config: config.Config{OperatorNamespace: "valetkey-system"}, Kube: &kube.Client{Dynamic: dyn},
		Registry: registry.New(), Stats: stats.New()}
	mux := http.NewServeMux()
	api.Register(mux)

	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "http://localhost:8082"+path, nil)
		req.Header.Set("X-Valetkey-Request", "1")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	rec := get("/api/v1/proxykeys/billing/invoice-api/yaml")
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	out := rec.Body.String()
	for _, want := range []string{"apiVersion: valetkey.dev/v1alpha1", "kind: ProxyKey", "name: invoice-api",
		"namespace: billing", "app: invoices", "rpm: 60", "- gpt-4o-mini"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"status", "uid", "resourceVersion", "managedFields", "last-applied", "annotations"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("%q should be left out:\n%s", unwanted, out)
		}
	}
	if code := get("/api/v1/proxykeys/billing/missing/yaml").Code; code != 404 {
		t.Errorf("missing key: %d", code)
	}
}

// A tenant's YAML is its whole file: Namespace, Group, then its keys by name.
func TestGroupYAML(t *testing.T) {
	obj := func(kind, ns, name string, spec map[string]any) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "valetkey.dev/v1alpha1", "kind": kind,
			"metadata": map[string]any{"name": name, "namespace": ns}, "spec": spec,
			"status": map[string]any{"ready": true},
		}}
	}
	dyn := dynfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{types.GroupGVR: "GroupList", types.ProxyKeyGVR: "ProxyKeyList"})
	create := func(gvr schema.GroupVersionResource, o *unstructured.Unstructured) {
		if _, err := dyn.Resource(gvr).Namespace(o.GetNamespace()).Create(t.Context(), o, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	create(types.GroupGVR, obj("Group", "valetkey-system", "billing", map[string]any{"channelRefs": []any{"openai"}}))
	create(types.ProxyKeyGVR, obj("ProxyKey", "billing", "zeta-app", map[string]any{"rpm": int64(5)}))
	create(types.ProxyKeyGVR, obj("ProxyKey", "billing", "alpha-app", map[string]any{}))
	create(types.ProxyKeyGVR, obj("ProxyKey", "support", "helpdesk", map[string]any{}))

	api := &API{Config: config.Config{OperatorNamespace: "valetkey-system"}, Kube: &kube.Client{Dynamic: dyn},
		Registry: registry.New(), Stats: stats.New()}
	mux := http.NewServeMux()
	api.Register(mux)
	req := httptest.NewRequest("GET", "http://localhost:8082/api/v1/groups/billing/yaml", nil)
	req.Header.Set("X-Valetkey-Request", "1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	out := rec.Body.String()
	docs := strings.Split(out, "---\n")
	if len(docs) != 4 {
		t.Fatalf("want Namespace, Group and 2 keys, got %d documents:\n%s", len(docs), out)
	}
	for i, want := range []string{"kind: Namespace", "kind: Group", "name: alpha-app", "name: zeta-app"} {
		if !strings.Contains(docs[i], want) {
			t.Errorf("document %d should contain %q:\n%s", i, want, docs[i])
		}
	}
	if strings.Contains(out, "helpdesk") || strings.Contains(out, "status") {
		t.Errorf("other tenants' keys and status must be left out:\n%s", out)
	}
}
