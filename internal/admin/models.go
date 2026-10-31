package admin

import (
	"context"
	"net/http"
	"slices"
	"sort"

	"github.com/anas1412/valetkey/internal/registry"
	"github.com/anas1412/valetkey/internal/relay"
	"github.com/anas1412/valetkey/internal/types"
)

// listResult is a provider's model list call: what the dashboard shows when picking models.
type listResult = relay.ModelList

// fetchModels calls the provider's GET /v1/models with its real key. No prompt is sent.
var fetchModels = relay.ListModels

// discoverModels lists a provider's models before it is saved (from the Add provider form).
func (a *API) discoverModels(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Spec     types.ProviderSpec `json:"spec"`
		APIKey   string             `json:"apiKey"`
		Provider string             `json:"provider"` // editing: use this provider's stored key when apiKey is empty
	}
	if !readJSON(w, r, &in) {
		return
	}
	key := in.APIKey
	if key == "" && in.Provider != "" {
		if e, ok := a.Registry.Provider(in.Provider); ok {
			key = e.UpstreamKey
		}
	}
	if key == "" {
		writeMessage(w, 400, "paste the API key first")
		return
	}
	res, err := fetchModels(r.Context(), &types.Provider{Spec: in.Spec}, key)
	if err != nil {
		writeMessage(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, res)
}

// tenantModels lists every model a tenant's keys can use, for the key form's model picker.
// Providers without a model list are asked for theirs.
func (a *API) tenantModels(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := a.Registry.Tenant(name); !ok {
		writeMessage(w, 404, "tenant not found")
		return
	}
	models := providerModels(r.Context(), a.Registry.TenantProviders(name))
	writeJSON(w, 200, map[string]any{"models": models})
}

func providerModels(ctx context.Context, providers []registry.ProviderEntry) []string {
	var all []string
	for _, e := range providers {
		ids := slices.Clone(e.Provider.Spec.Models)
		if len(ids) == 0 {
			if res, err := fetchModels(ctx, e.Provider, e.UpstreamKey); err == nil {
				ids = res.Models
			}
		}
		for m := range e.Provider.Spec.ModelMapping {
			ids = append(ids, m)
		}
		all = append(all, ids...)
	}
	sort.Strings(all)
	return slices.Compact(all)
}
