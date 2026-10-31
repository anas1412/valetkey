// Package auth authenticates tenants (virtual keys) and admins (session cookie or bearer key).
package auth

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/anas1412/llmp/internal/keys"
	"github.com/anas1412/llmp/internal/registry"
)

type ctxKey struct{}

// RouteFrom returns the route TokenAuth attached to the request.
func RouteFrom(ctx context.Context) registry.Route {
	r, _ := ctx.Value(ctxKey{}).(registry.Route)
	return r
}

// WithRoute attaches a route to a context (used by TokenAuth and tests).
func WithRoute(ctx context.Context, r registry.Route) context.Context {
	return context.WithValue(ctx, ctxKey{}, r)
}

// TokenAuth checks the virtual key (Bearer or x-api-key) and the key's IP allow list.
// trustProxy is the number of reverse proxies whose X-Forwarded-For entries are trusted.
func TokenAuth(reg *registry.Registry, trustProxy string, next http.Handler) http.Handler {
	hops, _ := strconv.Atoi(trustProxy)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := ""
		if v, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
			key = v
		} else {
			key = r.Header.Get("X-Api-Key")
		}
		if key == "" {
			unauthorized(w, r, "missing API key")
			return
		}
		route, ok := reg.Lookup(keys.Hash(key))
		if !ok {
			unauthorized(w, r, "invalid API key")
			return
		}
		if len(route.AllowList) > 0 && !route.AllowList.Allows(ClientIP(r, hops)) {
			unauthorized(w, r, "client IP not allowed for this key")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithRoute(r.Context(), route)))
	})
}

// ClientIP is the connection's address, or with hops > 0 the entry that many
// positions from the right of X-Forwarded-For (the address your proxy saw).
func ClientIP(r *http.Request, hops int) string {
	if hops > 0 {
		parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
		if i := len(parts) - hops; i >= 0 && i < len(parts) {
			if ip := strings.TrimSpace(parts[i]); ip != "" {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// unauthorized answers 401 in the error shape of the API being called.
func unauthorized(w http.ResponseWriter, r *http.Request, msg string) {
	var body any = map[string]any{"error": map[string]string{"message": msg, "type": "invalid_api_key", "code": "invalid_api_key"}}
	if strings.HasSuffix(r.URL.Path, "/messages") {
		body = map[string]any{"type": "error", "error": map[string]string{"type": "authentication_error", "message": msg}}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(body)
}
