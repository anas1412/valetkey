package admin

import (
	"net/http"
	"strings"
)

// Bucket size of the in-memory series.
const seriesStepMinutes = 5

// overview is everything the dashboard's Overview page needs in one call.
// With Prometheus configured, usage comes from it for ?range=24h|7d|30d and spans
// pod restarts; otherwise it's the pod's own memory (last 24 h since it started).
func (a *API) overview(w http.ResponseWriter, r *http.Request) {
	rangeName := r.URL.Query().Get("range")
	if _, ok := historyRanges[rangeName]; !ok {
		rangeName = "24h"
	}
	source, warning := "memory", ""
	history := memoryHistory(a.Stats)
	if a.Prom != nil {
		if h, err := promHistory(r.Context(), a.Prom, rangeName); err != nil {
			warning = "Prometheus unavailable, showing this pod's memory: " + err.Error()
		} else {
			history, source = h, "prometheus"
		}
	}
	if source == "memory" {
		rangeName = "24h"
	}

	type providerHealth struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Enabled bool   `json:"enabled"`
		Ready   bool   `json:"ready"`
		Message string `json:"message,omitempty"`
	}
	providers := []providerHealth{}
	for _, e := range a.Registry.Providers() {
		providers = append(providers, providerHealth{Name: e.Name(), Type: e.Provider.Spec.Type, Enabled: e.Provider.Enabled(),
			Ready: e.UpstreamKey != "", Message: e.Provider.Status.Message})
	}
	writeJSON(w, 200, map[string]any{
		"proxyURL":         a.Config.ProxyURL,
		"startedAt":        a.Stats.StartedAt,
		"source":           source,
		"range":            rangeName,
		"warning":          warning,
		"prometheusConfig": a.Prom != nil,
		"stepMinutes":      history.StepMinutes,
		"totals":           history.Totals,
		"series":           history.Series,
		"providerShare":    history.ProviderShare,
		"cost":             history.Cost,
		"counts": map[string]int{
			"providers": len(a.Registry.Providers()), "tenants": len(a.Registry.Tenants()),
			"keys": len(a.Controller.ProxyKeys()), "activeKeys": len(a.Registry.Routes()),
		},
		"providers": providers,
	})
}

func (a *API) activity(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"requests": a.Stats.Requests(), "events": a.Stats.Events()})
}

// namespaces lists namespaces for the tenant picker, flagging which already have a Tenant.
func (a *API) namespaces(w http.ResponseWriter, r *http.Request) {
	names, err := a.Kube.Namespaces(r.Context())
	if err != nil {
		writeKubeError(w, err)
		return
	}
	out := []map[string]any{}
	for _, n := range names {
		if strings.HasPrefix(n, "kube-") || n == a.Config.OperatorNamespace {
			continue
		}
		_, has := a.Registry.Tenant(n)
		out = append(out, map[string]any{"name": n, "hasTenant": has})
	}
	writeJSON(w, 200, out)
}
