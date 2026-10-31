package relay

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/anas1412/valetkey/internal/auth"
	"github.com/anas1412/valetkey/internal/metrics"
	"github.com/anas1412/valetkey/internal/registry"
	"github.com/anas1412/valetkey/internal/stats"
	"github.com/anas1412/valetkey/internal/types"
)

// A provider answering 401 has a bad key: the request moves on to the next provider
// and the bad one is reported, so it gets no more traffic.
func TestRejectedKeyFailsOver(t *testing.T) {
	var badHits atomic.Int32
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		badHits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":{"message":"invalid api key"}}`)
	}))
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"x","choices":[{"message":{"content":"hello"}}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`)
	}))
	t.Cleanup(func() { bad.Close(); good.Close() })

	reg := registry.New()
	add := func(name, url string, prio int) {
		reg.UpsertChannel(&types.Channel{ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: types.ChannelSpec{Type: "openai", BaseURL: url, Priority: prio, KeySecretRef: types.SecretRef{Name: "s"}}}, "key-"+name)
	}
	add("bad", bad.URL, 100) // preferred
	add("good", good.URL, 1)
	reg.UpsertGroup(&types.TenantGroup{ObjectMeta: metav1.ObjectMeta{Name: "team"}, Spec: types.GroupSpec{ChannelRefs: []string{"bad", "good"}}})

	var reported []string
	rl := &Relay{Registry: reg, Limiter: NewRateLimiter(), Metrics: metrics.New(), Stats: stats.New(), Transports: NewTransports(),
		OnKeyRejected: func(channel, reason string) {
			reported = append(reported, channel)
			reg.SetRejected(channel, reason) // what the controller does
		}}
	send := func() int {
		req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
		req = req.WithContext(auth.WithRoute(req.Context(), registry.Route{Group: "team", Namespace: "team", ProxyKeyName: "app"}))
		rec := httptest.NewRecorder()
		rl.ChatCompletions(rec, req)
		return rec.Code
	}

	if code := send(); code != 200 {
		t.Fatalf("first request: %d, want 200 from the next provider", code)
	}
	if len(reported) != 1 || reported[0] != "bad" {
		t.Fatalf("reported %v, want [bad]", reported)
	}
	if code := send(); code != 200 || badHits.Load() != 1 {
		t.Fatalf("second request: %d, bad provider hit %d times (want once)", code, badHits.Load())
	}
}
