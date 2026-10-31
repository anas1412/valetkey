// Package server builds the two HTTP listeners:
//   - proxy (8000): tenant-facing, serves only /v1/*
//   - admin (8081): dashboard, admin API, /healthz, /readyz, /metrics
package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"

	"github.com/anas1412/llmproxy-k8s/internal/admin"
	"github.com/anas1412/llmproxy-k8s/internal/auth"
	"github.com/anas1412/llmproxy-k8s/internal/metrics"
	"github.com/anas1412/llmproxy-k8s/internal/registry"
	"github.com/anas1412/llmproxy-k8s/internal/relay"
)

// Proxy is the tenant-facing handler.
func Proxy(rl *relay.Relay, reg *registry.Registry, trustProxy string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", rl.ChatCompletions)
	mux.HandleFunc("POST /v1/messages", rl.Messages)
	mux.HandleFunc("GET /v1/models", rl.Models)
	return withNotFound(auth.TokenAuth(reg, trustProxy, mux), func(r *http.Request) bool {
		return strings.HasPrefix(r.URL.Path, "/v1/")
	})
}

// Admin is the internal handler. dashboard may be nil (admin API only).
func Admin(api *admin.API, m *metrics.Metrics, reg *registry.Registry, ready func() bool, dashboard fs.FS) http.Handler {
	mux := http.NewServeMux()
	api.Register(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { writeHealth(w, 200, reg) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		// 503 until every informer has loaded its initial list.
		if ready() {
			writeHealth(w, 200, reg)
		} else {
			writeHealth(w, 503, reg)
		}
	})
	mux.Handle("GET /metrics", m.Handler())
	if dashboard != nil {
		mux.Handle("GET /", spa(dashboard))
	}
	return secureHeaders(mux)
}

// secureHeaders: no framing (clickjacking), no MIME sniffing, no referrer leaks,
// and scripts only from the dashboard itself.
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
			"img-src 'self' data:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

// spa serves the built dashboard; unknown paths get index.html so client-side routes work.
func spa(dist fs.FS) http.Handler {
	files := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" || path == "index.html" {
			serveIndex(w, dist)
			return
		}
		if _, err := fs.Stat(dist, path); err != nil {
			serveIndex(w, dist)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=3600")
		files.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, dist fs.FS) {
	b, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(b)
}

func writeHealth(w http.ResponseWriter, status int, reg *registry.Registry) {
	body := map[string]any{"ok": status == 200}
	for k, v := range reg.Stats() {
		body[k] = v
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// withNotFound answers JSON 404 for paths outside allowed, before auth runs.
func withNotFound(next http.Handler, allowed func(*http.Request) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowed(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"not found","type":"not_found"}}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
