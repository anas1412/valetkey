package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// Admin API against client-go's fake dynamic client: create/update write real objects.
func TestProviderCreateAndUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	dyn := dynfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		types.ProviderGVR: "ProviderList", types.TenantGVR: "TenantList", types.ProxyKeyGVR: "ProxyKeyList",
		types.SecretGVR: "SecretList", types.NamespaceGVR: "NamespaceList",
	})
	kc := &kube.Client{Dynamic: dyn}
	api := &API{Config: config.Config{OperatorNamespace: "valetkey-system"}, Kube: kc, Registry: registry.New(),
		Stats: stats.New()}
	mux := http.NewServeMux()
	api.Register(mux)

	call := func(method, path string, body any) int {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(method, "http://localhost:8082"+path, bytes.NewReader(b))
		req.Header.Set("X-Valetkey-Request", "1")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code >= 300 {
			t.Logf("%s %s → %d %s", method, path, rec.Code, rec.Body.String())
		}
		return rec.Code
	}

	spec := map[string]any{"type": "openai", "baseURL": "https://api.openai.com", "models": []string{"gpt-4o"}}
	if code := call("POST", "/api/v1/providers", map[string]any{"name": "openai", "spec": spec, "apiKey": "sk-real"}); code != 201 {
		t.Fatalf("create: %d", code)
	}
	ch, err := dyn.Resource(types.ProviderGVR).Namespace("valetkey-system").Get(t.Context(), "openai", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if ref, _, _ := unstructured.NestedString(ch.Object, "spec", "keySecretRef", "name"); ref != "openai-api-key" {
		t.Fatalf("keySecretRef = %q", ref)
	}
	if s, _ := kc.ReadSecret(t.Context(), "valetkey-system", "openai-api-key"); s == nil {
		t.Fatal("provider key Secret not created")
	}

	spec["models"] = []string{"gpt-4o", "gpt-4o-mini"}
	if code := call("PUT", "/api/v1/providers/openai", map[string]any{"spec": spec}); code != 200 {
		t.Fatalf("update: %d", code)
	}
	ch, _ = dyn.Resource(types.ProviderGVR).Namespace("valetkey-system").Get(t.Context(), "openai", metav1.GetOptions{})
	models, _, _ := unstructured.NestedStringSlice(ch.Object, "spec", "models")
	ref, _, _ := unstructured.NestedString(ch.Object, "spec", "keySecretRef", "name")
	if len(models) != 2 || ref != "openai-api-key" {
		t.Fatalf("after update models=%v ref=%q (key reference must be kept)", models, ref)
	}

	if code := call("POST", "/api/v1/providers", map[string]any{"name": "Bad Name", "spec": spec, "apiKey": "x"}); code != 400 {
		t.Fatalf("invalid name accepted: %d", code)
	}
}

// The admin API has no password, so it must refuse requests other websites could forge.
func TestGuardRejectsForeignRequests(t *testing.T) {
	api := &API{Registry: registry.New(), Stats: stats.New()}
	mux := http.NewServeMux()
	api.Register(mux)
	try := func(url string, header bool) int {
		req := httptest.NewRequest("GET", url, nil)
		if header {
			req.Header.Set("X-Valetkey-Request", "1")
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	for name, c := range map[string]struct {
		url    string
		header bool
		want   int
	}{
		"local with header":          {"http://localhost:8082/api/v1/providers", true, 200},
		"ipv4 loopback":              {"http://127.0.0.1:8082/api/v1/providers", true, 200},
		"ipv6 loopback":              {"http://[::1]:8082/api/v1/providers", true, 200},
		"no header (cross-site)":     {"http://localhost:8082/api/v1/providers", false, 403},
		"other host (DNS rebinding)": {"http://evil.example:8082/api/v1/providers", true, 403},
	} {
		if got := try(c.url, c.header); got != c.want {
			t.Errorf("%s: got %d, want %d", name, got, c.want)
		}
	}
}
