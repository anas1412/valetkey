package relay

import (
	"math/rand/v2"
	"slices"
	"sort"

	"github.com/anas1412/valetkey/internal/registry"
)

// Candidates returns the tenant's providers that can serve model for this client API,
// in attempt order: a priority-weighted random primary, then the rest by priority.
// A provider with priority 20 gets twice the traffic of one with 10.
// Anthropic-format clients can only be served by anthropic providers; embeddings and
// the Responses API only by OpenAI-compatible ones.
func Candidates(reg *registry.Registry, tenant, model string, api API) []registry.ProviderEntry {
	var eligible []registry.ProviderEntry
	for _, e := range reg.TenantProviders(tenant) {
		if !canServe(api, e.Provider.Spec.Type) {
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
		total += e.Provider.EffectivePriority()
	}
	r := rand.IntN(total)
	primary := len(eligible) - 1
	for i, e := range eligible {
		if r -= e.Provider.EffectivePriority(); r < 0 {
			primary = i
			break
		}
	}
	rest := slices.Delete(slices.Clone(eligible), primary, primary+1)
	sort.SliceStable(rest, func(i, j int) bool {
		return rest[i].Provider.EffectivePriority() > rest[j].Provider.EffectivePriority()
	})
	return append([]registry.ProviderEntry{eligible[primary]}, rest...)
}

// ServesModel: a provider with no model list serves anything; otherwise the model must be listed or mapped.
func ServesModel(e registry.ProviderEntry, model string) bool {
	spec := e.Provider.Spec
	if len(spec.Models) == 0 {
		return true
	}
	_, mapped := spec.ModelMapping[model]
	return slices.Contains(spec.Models, model) || mapped
}

// canServe: chat completions work on every provider (Anthropic ones translate), the
// other APIs only on providers that speak them natively.
func canServe(api API, providerType string) bool {
	switch api {
	case OpenAI:
		return true
	case Anthropic:
		return providerType == "anthropic"
	default:
		return providerType != "anthropic"
	}
}
