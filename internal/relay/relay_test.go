package relay

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/anas1412/valetkey/internal/auth"
	"github.com/anas1412/valetkey/internal/keys"
	"github.com/anas1412/valetkey/internal/metrics"
	"github.com/anas1412/valetkey/internal/registry"
	"github.com/anas1412/valetkey/internal/stats"
	"github.com/anas1412/valetkey/internal/types"
)

// Relay integration: a real Relay between an HTTP client and fake upstreams. No Kubernetes.

type seenRequest struct {
	path    string
	headers http.Header
	body    map[string]any
}

type fixture struct {
	proxy *httptest.Server
	stats *stats.Stats
	mu    sync.Mutex
	seen  []seenRequest
}

func (f *fixture) last() seenRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seen[len(f.seen)-1]
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{stats: stats.New()}
	record := func(r *http.Request) map[string]any {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.seen = append(f.seen, seenRequest{path: r.URL.Path, headers: r.Header.Clone(), body: body})
		f.mu.Unlock()
		return body
	}
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := record(r)
		if body["stream"] == true {
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"he\"}}]}\n\n")
			io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2}}\n\ndata: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "upstream=secret")
		io.WriteString(w, `{"id":"x","choices":[{"message":{"content":"hello"}}],"usage":{"prompt_tokens":10,"completion_tokens":4}}`)
	}))
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		io.WriteString(w, `{"error":{"message":"boom"}}`)
	}))
	claude := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := record(r)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "msg_1", "model": body["model"], "stop_reason": "end_turn",
			"content": []any{map[string]any{"type": "text", "text": "hi from claude"}}, "usage": map[string]int{"input_tokens": 6, "output_tokens": 2}})
	}))
	t.Cleanup(func() { ok.Close(); broken.Close(); claude.Close() })

	reg := registry.New()
	add := func(name, typ, url string, prio int, models ...string) {
		reg.UpsertChannel(&types.Channel{ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: types.ChannelSpec{Type: typ, BaseURL: url, Priority: prio, Models: models, KeySecretRef: types.SecretRef{Name: "s"}}}, "real-"+name+"-key")
	}
	add("ok", "openai", ok.URL, 1, "gpt-4o", "gpt-4o-mini")
	add("broken", "openai", broken.URL, 1_000_000, "gpt-4o", "only-broken")
	add("claude", "anthropic", claude.URL, 1, "claude-x")
	reg.UpsertGroup(&types.TenantGroup{ObjectMeta: metav1.ObjectMeta{Name: "team"}, Spec: types.GroupSpec{ChannelRefs: []string{"ok", "broken", "claude"}}})

	rl := &Relay{Registry: reg, Limiter: NewRateLimiter(), Metrics: metrics.New(), Stats: f.stats, Transports: NewTransports()}
	mux := http.NewServeMux()
	withRoute := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			route := registry.Route{Group: "team", Namespace: "team", ProxyKeyName: "app"}
			switch r.Header.Get("X-Test-Route") {
			case "mini-only":
				route.Models = []string{"gpt-4o-mini"}
			case "rpm1":
				route.ProxyKeyName, route.RPM = "limited", 1
			case "maxtok":
				route.MaxTokens = 77
			}
			h(w, r.WithContext(auth.WithRoute(r.Context(), route)))
		}
	}
	mux.HandleFunc("POST /v1/chat/completions", withRoute(rl.ChatCompletions))
	mux.HandleFunc("POST /v1/messages", withRoute(rl.Messages))
	mux.HandleFunc("POST /v1/embeddings", withRoute(rl.Embeddings))
	mux.HandleFunc("POST /v1/responses", withRoute(rl.Responses))
	f.proxy = httptest.NewServer(mux)
	t.Cleanup(f.proxy.Close)
	return f
}

func (f *fixture) post(t *testing.T, path string, body any, headers map[string]string) (*http.Response, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, f.proxy.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = map[string]any{"_raw": string(raw)}
	}
	return resp, out
}

func TestRelay(t *testing.T) {
	f := newFixture(t)

	t.Run("upstream key swapped in, client headers filtered, usage recorded", func(t *testing.T) {
		resp, body := f.post(t, "/v1/chat/completions", map[string]any{"model": "gpt-4o-mini", "messages": []any{}},
			map[string]string{"Authorization": "Bearer sk-proxy-tenant", "Cookie": "a=b", "Accept-Encoding": "gzip", "X-Forwarded-For": "1.2.3.4"})
		if resp.StatusCode != 200 || resp.Header.Get("Set-Cookie") != "" {
			t.Fatalf("status %d, set-cookie %q", resp.StatusCode, resp.Header.Get("Set-Cookie"))
		}
		if body["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)["content"] != "hello" {
			t.Fatalf("bad body %v", body)
		}
		up := f.last()
		if up.path != "/v1/chat/completions" || up.headers.Get("Authorization") != "Bearer real-ok-key" ||
			up.headers.Get("Cookie") != "" || up.headers.Get("Accept-Encoding") != "" || up.headers.Get("X-Forwarded-For") != "" {
			t.Fatalf("bad upstream request %s %v", up.path, up.headers)
		}
		if r := f.stats.Requests()[0]; r.Channel != "ok" || r.InputTokens != 10 || r.OutputTokens != 4 || r.Status != 200 {
			t.Fatalf("bad stats %+v", r)
		}
	})

	t.Run("streaming passes SSE through and counts tokens", func(t *testing.T) {
		resp, body := f.post(t, "/v1/chat/completions", map[string]any{"model": "gpt-4o-mini", "messages": []any{}, "stream": true}, nil)
		raw := body["_raw"].(string)
		if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") ||
			!strings.Contains(raw, `"content":"he"`) || !strings.Contains(raw, "[DONE]") {
			t.Fatalf("bad stream %d %q", resp.StatusCode, raw)
		}
		if r := f.stats.Requests()[0]; r.InputTokens != 3 || r.OutputTokens != 2 {
			t.Fatalf("bad stream usage %+v", r)
		}
	})

	t.Run("a 500 from the preferred channel is retried on the next one", func(t *testing.T) {
		resp, _ := f.post(t, "/v1/chat/completions", map[string]any{"model": "gpt-4o", "messages": []any{}}, nil)
		if resp.StatusCode != 200 || f.stats.Requests()[0].Channel != "ok" {
			t.Fatalf("status %d via %s", resp.StatusCode, f.stats.Requests()[0].Channel)
		}
	})

	t.Run("when every channel fails the last upstream error is returned", func(t *testing.T) {
		resp, body := f.post(t, "/v1/chat/completions", map[string]any{"model": "only-broken", "messages": []any{}}, nil)
		if resp.StatusCode != 500 || body["error"].(map[string]any)["message"] != "boom" {
			t.Fatalf("status %d body %v", resp.StatusCode, body)
		}
	})

	t.Run("OpenAI client on an Anthropic channel gets a translated response", func(t *testing.T) {
		resp, body := f.post(t, "/v1/chat/completions", map[string]any{"model": "claude-x",
			"messages": []any{map[string]any{"role": "system", "content": "s"}, map[string]any{"role": "user", "content": "hi"}}}, nil)
		if resp.StatusCode != 200 || body["object"] != "chat.completion" ||
			body["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)["content"] != "hi from claude" {
			t.Fatalf("status %d body %v", resp.StatusCode, body)
		}
		up := f.last()
		if up.path != "/v1/messages" || up.headers.Get("X-Api-Key") != "real-claude-key" || up.body["system"] != "s" {
			t.Fatalf("bad upstream %s %v %v", up.path, up.headers, up.body)
		}
	})

	t.Run("Anthropic client on /v1/messages is passed through natively", func(t *testing.T) {
		resp, body := f.post(t, "/v1/messages", map[string]any{"model": "claude-x", "system": "native", "max_tokens": 5,
			"messages": []any{map[string]any{"role": "user", "content": "hi"}}}, nil)
		if resp.StatusCode != 200 || body["content"].([]any)[0].(map[string]any)["text"] != "hi from claude" || f.last().body["system"] != "native" {
			t.Fatalf("status %d body %v", resp.StatusCode, body)
		}
	})

	t.Run("embeddings and responses go to the same path upstream, never to Anthropic channels", func(t *testing.T) {
		for _, path := range []string{"/v1/embeddings", "/v1/responses"} {
			resp, _ := f.post(t, path, map[string]any{"model": "gpt-4o-mini", "input": "hi"}, nil)
			if resp.StatusCode != 200 || f.last().path != path || f.last().headers.Get("Authorization") != "Bearer real-ok-key" {
				t.Fatalf("%s: status %d, upstream %s", path, resp.StatusCode, f.last().path)
			}
		}
		resp, body := f.post(t, "/v1/embeddings", map[string]any{"model": "claude-x", "input": "hi"}, nil)
		if resp.StatusCode != http.StatusServiceUnavailable || body["error"].(map[string]any)["type"] != "no_channel_available" {
			t.Fatalf("embeddings on an anthropic-only model: %d %v", resp.StatusCode, body)
		}
	})

	t.Run("the key's default max_tokens is added when the request has none", func(t *testing.T) {
		hdr := map[string]string{"X-Test-Route": "maxtok"}
		f.post(t, "/v1/chat/completions", map[string]any{"model": "gpt-4o-mini", "messages": []any{}}, hdr)
		if f.last().body["max_tokens"] != 77.0 {
			t.Fatalf("openai-compatible body %v", f.last().body)
		}
		f.post(t, "/v1/chat/completions", map[string]any{"model": "claude-x", "messages": []any{map[string]any{"role": "user", "content": "hi"}}}, hdr)
		if f.last().body["max_tokens"] != 77.0 {
			t.Fatalf("translated anthropic body %v", f.last().body)
		}
		f.post(t, "/v1/responses", map[string]any{"model": "gpt-4o-mini", "input": "hi", "max_output_tokens": 5}, hdr)
		if f.last().body["max_output_tokens"] != 5.0 {
			t.Fatalf("the request's own cap must win: %v", f.last().body)
		}
		if got := maxTokensField(OpenAI, types.ChannelSpec{BaseURL: "https://api.openai.com"}); got != "max_completion_tokens" {
			t.Fatalf("official OpenAI field %q", got)
		}
	})

	t.Run("client errors are answered immediately with JSON", func(t *testing.T) {
		resp, body := f.post(t, "/v1/chat/completions", map[string]any{"model": "nobody-serves-this", "messages": []any{}}, nil)
		if resp.StatusCode != 503 || body["error"].(map[string]any)["type"] != "no_channel_available" {
			t.Fatalf("status %d body %v", resp.StatusCode, body)
		}
		bad, _ := http.Post(f.proxy.URL+"/v1/chat/completions", "application/json", strings.NewReader("{nope"))
		if bad.StatusCode != 400 {
			t.Fatalf("invalid JSON: %d", bad.StatusCode)
		}
		resp, body = f.post(t, "/v1/messages", map[string]any{"model": "gpt-4o", "messages": []any{}}, nil)
		if resp.StatusCode != 503 || body["type"] != "error" {
			t.Fatalf("anthropic-shaped error: %d %v", resp.StatusCode, body)
		}
	})

	t.Run("key model allowlist is enforced", func(t *testing.T) {
		resp, body := f.post(t, "/v1/chat/completions", map[string]any{"model": "gpt-4o", "messages": []any{}}, map[string]string{"X-Test-Route": "mini-only"})
		if resp.StatusCode != 403 || body["error"].(map[string]any)["type"] != "model_not_allowed" {
			t.Fatalf("status %d body %v", resp.StatusCode, body)
		}
	})

	t.Run("per-key RPM limit returns 429 with retry-after", func(t *testing.T) {
		h := map[string]string{"X-Test-Route": "rpm1"}
		if resp, _ := f.post(t, "/v1/chat/completions", map[string]any{"model": "gpt-4o-mini", "messages": []any{}}, h); resp.StatusCode != 200 {
			t.Fatalf("first request: %d", resp.StatusCode)
		}
		resp, _ := f.post(t, "/v1/chat/completions", map[string]any{"model": "gpt-4o-mini", "messages": []any{}}, h)
		if resp.StatusCode != 429 || resp.Header.Get("Retry-After") == "" {
			t.Fatalf("second request: %d retry-after %q", resp.StatusCode, resp.Header.Get("Retry-After"))
		}
	})
}

func TestTokenAuth(t *testing.T) {
	reg := registry.New()
	allow, _ := keys.ParseAllowList([]string{"10.0.0.0/8"})
	reg.UpsertRoute("uid", registry.Route{KeyHash: keys.Hash("sk-proxy-good"), Group: "team", AllowList: allow})
	reg.UpsertRoute("old", registry.Route{KeyHash: keys.Hash("sk-proxy-old"), Group: "team", ExpiresAt: time.Now().Add(-time.Second)})
	h := auth.TokenAuth(reg, "", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))

	try := func(header, value, remote string) int {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		if header != "" {
			req.Header.Set(header, value)
		}
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	for name, c := range map[string]struct {
		header, value, remote string
		want                  int
	}{
		"missing key": {"", "", "10.0.0.1:1", 401},
		"unknown key": {"Authorization", "Bearer sk-proxy-bad", "10.0.0.1:1", 401},
		"wrong IP":    {"Authorization", "Bearer sk-proxy-good", "8.8.8.8:1", 401},
		"valid":       {"X-Api-Key", "sk-proxy-good", "[::ffff:10.0.0.1]:1", 204},
		"expired":     {"X-Api-Key", "sk-proxy-old", "10.0.0.1:1", 401},
	} {
		if got := try(c.header, c.value, c.remote); got != c.want {
			t.Errorf("%s: got %d, want %d", name, got, c.want)
		}
	}
}
