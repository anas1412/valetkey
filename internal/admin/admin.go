// Package admin serves the management API used by the dashboard.
// CRDs stay the source of truth: handlers write CRs and Secrets, and the
// informers update the registry.
package admin

import (
	"encoding/json"
	"errors"
	"github.com/anas1412/valetkey/internal/budget"
	"github.com/anas1412/valetkey/internal/pricing"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/anas1412/valetkey/internal/config"
	"github.com/anas1412/valetkey/internal/kube"
	"github.com/anas1412/valetkey/internal/promql"
	"github.com/anas1412/valetkey/internal/reconciler"
	"github.com/anas1412/valetkey/internal/registry"
	"github.com/anas1412/valetkey/internal/stats"
)

type API struct {
	Config     config.Config
	Kube       *kube.Client
	Registry   *registry.Registry
	Controller *reconciler.Controller
	Stats      *stats.Stats
	// Prom is optional: when set, usage history is read from Prometheus.
	Prom    *promql.Client
	Prices  *pricing.Store  // optional
	Budgets *budget.Tracker // optional
}

// Register mounts every /api/v1 route on mux.
func (a *API) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/overview", a.guard(a.overview))
	mux.Handle("GET /api/v1/activity", a.guard(a.activity))
	mux.Handle("GET /api/v1/namespaces", a.guard(a.namespaces))
	mux.Handle("GET /api/v1/pricing", a.guard(a.pricing))

	mux.Handle("GET /api/v1/providers", a.guard(a.listProviders))
	mux.Handle("POST /api/v1/providers", a.guard(a.createProvider))
	mux.Handle("GET /api/v1/providers/{name}", a.guard(a.getProvider))
	mux.Handle("PUT /api/v1/providers/{name}", a.guard(a.updateProvider))
	mux.Handle("DELETE /api/v1/providers/{name}", a.guard(a.deleteProvider))
	mux.Handle("POST /api/v1/providers/{name}/test", a.guard(a.testProvider))
	mux.Handle("GET /api/v1/providers/{name}/yaml", a.guard(a.providerYAML))
	mux.Handle("POST /api/v1/models/discover", a.guard(a.discoverModels))

	mux.Handle("GET /api/v1/tenants", a.guard(a.listTenants))
	mux.Handle("POST /api/v1/tenants", a.guard(a.createTenant))
	mux.Handle("GET /api/v1/tenants/{name}", a.guard(a.getTenant))
	mux.Handle("PUT /api/v1/tenants/{name}", a.guard(a.updateTenant))
	mux.Handle("DELETE /api/v1/tenants/{name}", a.guard(a.deleteTenant))
	mux.Handle("GET /api/v1/tenants/{name}/models", a.guard(a.tenantModels))
	mux.Handle("GET /api/v1/tenants/{name}/yaml", a.guard(a.tenantYAML))

	mux.Handle("GET /api/v1/proxykeys", a.guard(a.listKeys))
	mux.Handle("POST /api/v1/proxykeys", a.guard(a.createKey))
	mux.Handle("GET /api/v1/proxykeys/{ns}/{name}", a.guard(a.getKey))
	mux.Handle("PUT /api/v1/proxykeys/{ns}/{name}", a.guard(a.updateKey))
	mux.Handle("DELETE /api/v1/proxykeys/{ns}/{name}", a.guard(a.deleteKey))
	mux.Handle("GET /api/v1/proxykeys/{ns}/{name}/key", a.guard(a.revealKey))
	mux.Handle("POST /api/v1/proxykeys/{ns}/{name}/rotate", a.guard(a.rotateKey))
	mux.Handle("GET /api/v1/proxykeys/{ns}/{name}/yaml", a.guard(a.keyYAML))
}

// guard admits only requests the dashboard or a local script would send. The admin
// API has no password: it listens on the pod's loopback, so reaching it at all takes
// `kubectl port-forward` (Kubernetes RBAC). These checks stop other websites from
// using an admin's open port-forward:
//   - Host must be localhost: blocks DNS-rebinding pages.
//   - X-Valetkey-Request must be set: a cross-site page can't add a custom header
//     without a CORS preflight, which this API never grants.
func (a *API) guard(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !LocalHost(r.Host) {
			writeMessage(w, http.StatusForbidden, "the admin API only answers on localhost (use kubectl port-forward)")
			return
		}
		if r.Header.Get("X-Valetkey-Request") == "" {
			writeMessage(w, http.StatusForbidden, "missing X-Valetkey-Request header")
			return
		}
		h(w, r)
	})
}

// LocalHost reports whether a Host header names this machine (any port).
func LocalHost(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// audit records an admin change in the activity log and the pod log (which outlives
// restarts). Who made it is in the Kubernetes audit log, as the port-forward's user.
func (a *API) audit(_ *http.Request, action string) {
	slog.Info("admin action", "action", action)
	a.Stats.Event("info", "admin", action)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeMessage(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"message": msg})
}

// readJSON decodes a JSON body (max 1 MiB) or answers 400.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		writeMessage(w, http.StatusBadRequest, "request body must be JSON")
		return false
	}
	return true
}

// writeKubeError turns a Kubernetes API error into an HTTP error carrying its message
// (schema validation, already exists, not found…).
func writeKubeError(w http.ResponseWriter, err error) {
	var status apierrors.APIStatus
	if errors.As(err, &status) {
		code := int(status.Status().Code)
		if code < 400 || code > 599 {
			code = http.StatusBadGateway
		}
		writeMessage(w, code, status.Status().Message)
		return
	}
	writeMessage(w, http.StatusBadGateway, err.Error())
}

var dnsName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]{0,251}[a-z0-9])?$`)

// validName answers 400 unless name is a valid Kubernetes object name.
func validName(w http.ResponseWriter, name, what string) bool {
	if dnsName.MatchString(name) {
		return true
	}
	writeMessage(w, http.StatusBadRequest, what+` must be lowercase letters, digits, "-" or "." (e.g. "my-team")`)
	return false
}
