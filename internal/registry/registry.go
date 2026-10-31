// Package registry is the in-memory routing table built from informer events.
// Nothing is persisted: it is rebuilt from the informers' initial lists on start.
package registry

import (
	"github.com/anas1412/valetkey/internal/budget"
	"sync"
	"time"

	"github.com/anas1412/valetkey/internal/keys"
	"github.com/anas1412/valetkey/internal/types"
)

// Route maps a hashed virtual key to its tenant and limits.
type Route struct {
	KeyHash      string
	Tenant       string
	Namespace    string
	ProxyKeyName string
	Models       []string // nil = any
	AllowList    keys.AllowList
	RPM, TPM     int
	Budget       *budget.Limit // nil = no budget
	ModelBudgets map[string]*budget.Limit
	MaxTokens    int       // default output cap, 0 = none
	ExpiresAt    time.Time // zero = never
}

// ProviderEntry is a Provider plus its resolved upstream key ("" = unresolved).
type ProviderEntry struct {
	Provider    *types.Provider
	UpstreamKey string
	// Rejected is why the provider refused the key (HTTP 401/403), "" if it hasn't.
	Rejected string
}

func (e ProviderEntry) Name() string { return e.Provider.Name }

// Usable means enabled, with a resolved key the provider hasn't rejected.
func (e ProviderEntry) Usable() bool {
	return e.Provider.Enabled() && e.UpstreamKey != "" && e.Rejected == ""
}

// SameKey reports whether the provider still sends the same key to the same provider,
// so an earlier verdict on the key still holds.
func (e ProviderEntry) SameKey(ch *types.Provider, upstreamKey string) bool {
	return e.UpstreamKey == upstreamKey && e.Provider.Spec.BaseURL == ch.Spec.BaseURL && e.Provider.Spec.Type == ch.Spec.Type
}

type Registry struct {
	mu        sync.RWMutex
	routes    map[string]Route  // keyHash → route
	byUID     map[string]string // ProxyKey uid → keyHash
	providers map[string]ProviderEntry
	tenants   map[string]*types.Tenant
}

func New() *Registry {
	return &Registry{
		routes: map[string]Route{}, byUID: map[string]string{},
		providers: map[string]ProviderEntry{}, tenants: map[string]*types.Tenant{},
	}
}

func (r *Registry) UpsertRoute(uid string, route Route) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old, ok := r.byUID[uid]; ok && old != route.KeyHash {
		delete(r.routes, old)
	}
	r.byUID[uid] = route.KeyHash
	r.routes[route.KeyHash] = route
}

func (r *Registry) EvictRoute(uid string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if h, ok := r.byUID[uid]; ok {
		delete(r.routes, h)
		delete(r.byUID, uid)
	}
}

// Lookup is called on every request.
func (r *Registry) Lookup(keyHash string) (Route, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	route, ok := r.routes[keyHash]
	return route, ok
}

func (r *Registry) Routes() []Route {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Route, 0, len(r.routes))
	for _, v := range r.routes {
		out = append(out, v)
	}
	return out
}

func (r *Registry) UpsertProvider(ch *types.Provider, upstreamKey string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := ProviderEntry{Provider: ch, UpstreamKey: upstreamKey}
	if old, ok := r.providers[ch.Name]; ok && old.SameKey(ch, upstreamKey) {
		e.Rejected = old.Rejected
	}
	r.providers[ch.Name] = e
}

// SetRejected records (msg != "") or clears a provider's refusal of the provider's key.
// It reports whether anything changed.
func (r *Registry) SetRejected(name, msg string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.providers[name]
	if !ok || e.Rejected == msg {
		return false
	}
	e.Rejected = msg
	r.providers[name] = e
	return true
}

func (r *Registry) EvictProvider(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.providers, name)
}

func (r *Registry) Provider(name string) (ProviderEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.providers[name]
	return e, ok
}

func (r *Registry) Providers() []ProviderEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ProviderEntry, 0, len(r.providers))
	for _, v := range r.providers {
		out = append(out, v)
	}
	return out
}

func (r *Registry) UpsertTenant(g *types.Tenant) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tenants[g.Name] = g
}

func (r *Registry) EvictTenant(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tenants, name)
}

func (r *Registry) Tenant(name string) (*types.Tenant, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.tenants[name]
	return g, ok
}

func (r *Registry) Tenants() []*types.Tenant {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*types.Tenant, 0, len(r.tenants))
	for _, v := range r.tenants {
		out = append(out, v)
	}
	return out
}

// TenantProviders returns the tenant's usable providers, in providers order.
func (r *Registry) TenantProviders(tenant string) []ProviderEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.tenants[tenant]
	if !ok {
		return nil
	}
	var out []ProviderEntry
	for _, name := range g.Spec.Providers {
		if e, ok := r.providers[name]; ok && e.Usable() {
			out = append(out, e)
		}
	}
	return out
}

func (r *Registry) Stats() map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return map[string]int{"routes": len(r.routes), "providers": len(r.providers), "tenants": len(r.tenants)}
}
