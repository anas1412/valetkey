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

	type channelHealth struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Enabled bool   `json:"enabled"`
		Ready   bool   `json:"ready"`
		Message string `json:"message,omitempty"`
	}
	channels := []channelHealth{}
	for _, e := range a.Registry.Channels() {
		channels = append(channels, channelHealth{Name: e.Name(), Type: e.Channel.Spec.Type, Enabled: e.Channel.Enabled(),
			Ready: e.UpstreamKey != "", Message: e.Channel.Status.Message})
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
		"channelShare":     history.ChannelShare,
		"counts": map[string]int{
			"channels": len(a.Registry.Channels()), "groups": len(a.Registry.Groups()),
			"keys": len(a.Controller.ProxyKeys()), "activeKeys": len(a.Registry.Routes()),
		},
		"channels": channels,
	})
}

func (a *API) activity(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"requests": a.Stats.Requests(), "events": a.Stats.Events()})
}

// status is the compact summary used by valetkeyctl.
func (a *API) status(w http.ResponseWriter, _ *http.Request) {
	channels, groups, keys := []any{}, []any{}, []any{}
	for _, e := range a.Registry.Channels() {
		models := e.Channel.Spec.Models
		if models == nil {
			models = []string{}
		}
		channels = append(channels, map[string]any{"name": e.Name(), "type": e.Channel.Spec.Type, "models": models, "ready": e.UpstreamKey != ""})
	}
	for _, g := range a.Registry.Groups() {
		st := g.Spec.Status
		if st == "" {
			st = "Enabled"
		}
		groups = append(groups, map[string]any{"name": g.Name, "status": st, "channels": g.Spec.ChannelRefs})
	}
	for _, pk := range a.Controller.ProxyKeys() {
		keys = append(keys, map[string]any{"namespace": pk.Namespace, "name": pk.Name, "group": pk.Namespace, "ready": pk.Status.Ready})
	}
	writeJSON(w, 200, map[string]any{"channels": channels, "groups": groups, "keys": keys})
}

// namespaces lists namespaces for the tenant picker, flagging which already have a Group.
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
		_, has := a.Registry.Group(n)
		out = append(out, map[string]any{"name": n, "hasGroup": has})
	}
	writeJSON(w, 200, out)
}
