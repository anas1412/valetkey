// Package admin serves the management API used by the dashboard and llmproxyctl.
// CRDs stay the source of truth: handlers write CRs and Secrets, and the
// informers update the registry.
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/anas1412/llmp/internal/auth"
	"github.com/anas1412/llmp/internal/config"
	"github.com/anas1412/llmp/internal/kube"
	"github.com/anas1412/llmp/internal/reconciler"
	"github.com/anas1412/llmp/internal/registry"
	"github.com/anas1412/llmp/internal/stats"
)

type API struct {
	Config     config.Config
	Kube       *kube.Client
	Registry   *registry.Registry
	Controller *reconciler.Controller
	Stats      *stats.Stats
	Sessions   *auth.Sessions
}

// Register mounts every /api/v1 route on mux.
func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/session", a.login)
	mux.HandleFunc("DELETE /api/v1/session", a.logout)
	mux.Handle("GET /api/v1/session", a.guard(func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]bool{"ok": true}) }))

	mux.Handle("GET /api/v1/overview", a.guard(a.overview))
	mux.Handle("GET /api/v1/activity", a.guard(a.activity))
	mux.Handle("GET /api/v1/status", a.guard(a.status))
	mux.Handle("GET /api/v1/namespaces", a.guard(a.namespaces))

	mux.Handle("GET /api/v1/channels", a.guard(a.listChannels))
	mux.Handle("POST /api/v1/channels", a.guard(a.createChannel))
	mux.Handle("GET /api/v1/channels/{name}", a.guard(a.getChannel))
	mux.Handle("PUT /api/v1/channels/{name}", a.guard(a.updateChannel))
	mux.Handle("DELETE /api/v1/channels/{name}", a.guard(a.deleteChannel))
	mux.Handle("POST /api/v1/channels/{name}/test", a.guard(a.testChannel))
	mux.Handle("POST /api/v1/models/discover", a.guard(a.discoverModels))

	mux.Handle("GET /api/v1/groups", a.guard(a.listGroups))
	mux.Handle("POST /api/v1/groups", a.guard(a.createGroup))
	mux.Handle("GET /api/v1/groups/{name}", a.guard(a.getGroup))
	mux.Handle("PUT /api/v1/groups/{name}", a.guard(a.updateGroup))
	mux.Handle("DELETE /api/v1/groups/{name}", a.guard(a.deleteGroup))
	mux.Handle("GET /api/v1/groups/{name}/models", a.guard(a.groupModels))

	mux.Handle("GET /api/v1/proxykeys", a.guard(a.listKeys))
	mux.Handle("POST /api/v1/proxykeys", a.guard(a.createKey))
	mux.Handle("GET /api/v1/proxykeys/{ns}/{name}", a.guard(a.getKey))
	mux.Handle("PUT /api/v1/proxykeys/{ns}/{name}", a.guard(a.updateKey))
	mux.Handle("DELETE /api/v1/proxykeys/{ns}/{name}", a.guard(a.deleteKey))
	mux.Handle("GET /api/v1/proxykeys/{ns}/{name}/key", a.guard(a.revealKey))
	mux.Handle("POST /api/v1/proxykeys/{ns}/{name}/rotate", a.guard(a.rotateKey))
}

type viaKey struct{}

func (a *API) guard(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.Sessions.AdminKey == "" {
			writeMessage(w, http.StatusServiceUnavailable, "admin API disabled: ADMIN_KEY is not set")
			return
		}
		ok, via := a.Sessions.Authorized(r)
		if !ok {
			writeMessage(w, http.StatusUnauthorized, "admin authentication required")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), viaKey{}, via)))
	})
}

// audit records an admin change in the activity log and the pod log (which outlives restarts).
func (a *API) audit(r *http.Request, action string) {
	via, _ := r.Context().Value(viaKey{}).(string)
	ip := auth.ClientIP(r, 0)
	slog.Info("admin action", "action", action, "via", via, "ip", ip)
	a.Stats.Event("info", "admin", fmt.Sprintf("%s (via %s from %s)", action, via, ip))
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	if a.Sessions.AdminKey == "" {
		writeMessage(w, http.StatusServiceUnavailable, "admin API disabled: ADMIN_KEY is not set")
		return
	}
	ip := auth.ClientIP(r, 0)
	if wait := a.Sessions.LockedOut(ip); wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		writeMessage(w, http.StatusTooManyRequests, fmt.Sprintf("too many wrong keys, try again in %d min", int(wait.Minutes())+1))
		return
	}
	var body struct{ Key string }
	if !readJSON(w, r, &body) {
		return
	}
	if !a.Sessions.Login(w, r, body.Key, ip) {
		slog.Warn("admin sign-in failed", "ip", ip)
		a.Stats.Event("warn", "admin", "failed dashboard sign-in from "+ip)
		time.Sleep(500 * time.Millisecond) // slow down guessing
		writeMessage(w, http.StatusUnauthorized, "wrong admin key")
		return
	}
	slog.Info("admin signed in", "ip", ip)
	a.Stats.Event("info", "admin", "dashboard sign-in from "+ip)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	a.Sessions.Logout(w, r)
	w.WriteHeader(http.StatusNoContent)
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
