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
		Spec    types.ChannelSpec `json:"spec"`
		APIKey  string            `json:"apiKey"`
		Channel string            `json:"channel"` // editing: use this channel's stored key when apiKey is empty
	}
	if !readJSON(w, r, &in) {
		return
	}
	key := in.APIKey
	if key == "" && in.Channel != "" {
		if e, ok := a.Registry.Channel(in.Channel); ok {
			key = e.UpstreamKey
		}
	}
	if key == "" {
		writeMessage(w, 400, "paste the API key first")
		return
	}
	res, err := fetchModels(r.Context(), &types.Channel{Spec: in.Spec}, key)
	if err != nil {
		writeMessage(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, res)
}

// groupModels lists every model a tenant's keys can use, for the key form's model picker.
// Channels without a model list are asked for theirs.
func (a *API) groupModels(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := a.Registry.Group(name); !ok {
		writeMessage(w, 404, "group not found")
		return
	}
	models := channelModels(r.Context(), a.Registry.GroupChannels(name))
	writeJSON(w, 200, map[string]any{"models": models})
}

func channelModels(ctx context.Context, channels []registry.ChannelEntry) []string {
	var all []string
	for _, e := range channels {
		ids := slices.Clone(e.Channel.Spec.Models)
		if len(ids) == 0 {
			if res, err := fetchModels(ctx, e.Channel, e.UpstreamKey); err == nil {
				ids = res.Models
			}
		}
		for m := range e.Channel.Spec.ModelMapping {
			ids = append(ids, m)
		}
		all = append(all, ids...)
	}
	sort.Strings(all)
	return slices.Compact(all)
}
