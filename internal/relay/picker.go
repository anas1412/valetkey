package relay

import (
	"math/rand/v2"
	"slices"
	"sort"

	"github.com/anas1412/llmproxy-k8s/internal/registry"
)

// Candidates returns the group's channels that can serve model for this client API,
// in attempt order: a priority-weighted random primary, then the rest by priority.
// A channel with priority 20 gets twice the traffic of one with 10.
// Anthropic-format clients can only be served by anthropic channels.
func Candidates(reg *registry.Registry, group, model string, api API) []registry.ChannelEntry {
	var eligible []registry.ChannelEntry
	for _, e := range reg.GroupChannels(group) {
		if api == Anthropic && e.Channel.Spec.Type != "anthropic" {
			continue
		}
		if ServesModel(e, model) {
			eligible = append(eligible, e)
		}
	}
	if len(eligible) <= 1 {
		return eligible
	}

	total := 0
	for _, e := range eligible {
		total += e.Channel.EffectivePriority()
	}
	r := rand.IntN(total)
	primary := len(eligible) - 1
	for i, e := range eligible {
		if r -= e.Channel.EffectivePriority(); r < 0 {
			primary = i
			break
		}
	}
	rest := slices.Delete(slices.Clone(eligible), primary, primary+1)
	sort.SliceStable(rest, func(i, j int) bool {
		return rest[i].Channel.EffectivePriority() > rest[j].Channel.EffectivePriority()
	})
	return append([]registry.ChannelEntry{eligible[primary]}, rest...)
}

// ServesModel: a channel with no model list serves anything; otherwise the model must be listed or mapped.
func ServesModel(e registry.ChannelEntry, model string) bool {
	spec := e.Channel.Spec
	if len(spec.Models) == 0 {
		return true
	}
	_, mapped := spec.ModelMapping[model]
	return slices.Contains(spec.Models, model) || mapped
}
