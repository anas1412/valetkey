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

	"github.com/anas1412/llmp/internal/auth"
	"github.com/anas1412/llmp/internal/config"
	"github.com/anas1412/llmp/internal/kube"
	"github.com/anas1412/llmp/internal/registry"
	"github.com/anas1412/llmp/internal/stats"
	"github.com/anas1412/llmp/internal/types"
)

// Admin API against client-go's fake dynamic client: create/update write real objects.
func TestChannelCreateAndUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	dyn := dynfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		types.ChannelGVR: "ChannelList", types.GroupGVR: "GroupList", types.ProxyKeyGVR: "ProxyKeyList",
		types.SecretGVR: "SecretList", types.NamespaceGVR: "NamespaceList",
	})
	kc := &kube.Client{Dynamic: dyn}
	api := &API{Config: config.Config{OperatorNamespace: "llmproxy-system"}, Kube: kc, Registry: registry.New(),
		Stats: stats.New(), Sessions: auth.NewSessions("admin")}
	mux := http.NewServeMux()
	api.Register(mux)

	call := func(method, path string, body any) int {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer admin")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code >= 300 {
			t.Logf("%s %s → %d %s", method, path, rec.Code, rec.Body.String())
		}
		return rec.Code
	}

	spec := map[string]any{"type": "openai", "baseURL": "https://api.openai.com", "models": []string{"gpt-4o"}}
	if code := call("POST", "/api/v1/channels", map[string]any{"name": "openai", "spec": spec, "apiKey": "sk-real"}); code != 201 {
		t.Fatalf("create: %d", code)
	}
	ch, err := dyn.Resource(types.ChannelGVR).Namespace("llmproxy-system").Get(t.Context(), "openai", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if ref, _, _ := unstructured.NestedString(ch.Object, "spec", "keySecretRef", "name"); ref != "openai-api-key" {
		t.Fatalf("keySecretRef = %q", ref)
	}
	if s, _ := kc.ReadSecret(t.Context(), "llmproxy-system", "openai-api-key"); s == nil {
		t.Fatal("provider key Secret not created")
	}

	spec["models"] = []string{"gpt-4o", "gpt-4o-mini"}
	if code := call("PUT", "/api/v1/channels/openai", map[string]any{"spec": spec}); code != 200 {
		t.Fatalf("update: %d", code)
	}
	ch, _ = dyn.Resource(types.ChannelGVR).Namespace("llmproxy-system").Get(t.Context(), "openai", metav1.GetOptions{})
	models, _, _ := unstructured.NestedStringSlice(ch.Object, "spec", "models")
	ref, _, _ := unstructured.NestedString(ch.Object, "spec", "keySecretRef", "name")
	if len(models) != 2 || ref != "openai-api-key" {
		t.Fatalf("after update models=%v ref=%q (key reference must be kept)", models, ref)
	}

	if code := call("POST", "/api/v1/channels", map[string]any{"name": "Bad Name", "spec": spec, "apiKey": "x"}); code != 400 {
		t.Fatalf("invalid name accepted: %d", code)
	}
}
