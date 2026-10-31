package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"time"

	"github.com/anas1412/llmp/internal/registry"
	"github.com/anas1412/llmp/internal/relay"
	"github.com/anas1412/llmp/internal/types"
)

const discoverTimeout = 10 * time.Second

// listResult is a provider's model list call: what the dashboard shows when picking models.
type listResult struct {
	Models    []string `json:"models"`
	Status    int      `json:"status,omitempty"`
	LatencyMs int64    `json:"latencyMs"`
}

// fetchModels calls the provider's GET /v1/models with its real key. No prompt is sent.
func fetchModels(ctx context.Context, ch *types.Channel, apiKey string) (listResult, error) {
	var res listResult
	url, err := relay.JoinURL(ch.Spec.BaseURL, "/v1/models")
	if err != nil {
		return res, err
	}
	ctx, cancel := context.WithTimeout(ctx, discoverTimeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if ch.Spec.Type == "anthropic" {
		req.Header.Set("X-Api-Key", apiKey)
		req.Header.Set("Anthropic-Version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	start := time.Now()
	resp, err := relay.NewTransports().Client(ch).Do(req)
	res.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		return res, err
	}
	defer resp.Body.Close()
	res.Status = resp.StatusCode
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return res, fmt.Errorf("provider rejected the API key (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode >= 300 {
		return res, fmt.Errorf("provider answered HTTP %d", resp.StatusCode)
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil {
		return res, fmt.Errorf("provider's model list is not in the expected format")
	}
	for _, m := range body.Data {
		if m.ID != "" {
			res.Models = append(res.Models, m.ID)
		}
	}
	sort.Strings(res.Models)
	return res, nil
}

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
