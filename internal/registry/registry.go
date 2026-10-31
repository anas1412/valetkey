// Package registry is the in-memory routing table built from informer events.
// Nothing is persisted: it is rebuilt from the informers' initial lists on start.
package registry

import (
	"github.com/anas1412/valetkey/internal/budget"
	"sync"

	"github.com/anas1412/valetkey/internal/keys"
	"github.com/anas1412/valetkey/internal/types"
)

// Route maps a hashed virtual key to its tenant and limits.
type Route struct {
	KeyHash      string
	Group        string
	Namespace    string
	ProxyKeyName string
	Models       []string // nil = any
	AllowList    keys.AllowList
	RPM, TPM     int
	Budget       *budget.Limit // nil = no budget
}

// ChannelEntry is a Channel plus its resolved upstream key ("" = unresolved).
type ChannelEntry struct {
	Channel     *types.Channel
	UpstreamKey string
}

func (e ChannelEntry) Name() string { return e.Channel.Name }

// Usable means enabled and with a resolved key.
func (e ChannelEntry) Usable() bool { return e.Channel.Enabled() && e.UpstreamKey != "" }

type Registry struct {
	mu       sync.RWMutex
	routes   map[string]Route  // keyHash → route
	byUID    map[string]string // ProxyKey uid → keyHash
	channels map[string]ChannelEntry
	groups   map[string]*types.TenantGroup
}

func New() *Registry {
	return &Registry{
		routes: map[string]Route{}, byUID: map[string]string{},
		channels: map[string]ChannelEntry{}, groups: map[string]*types.TenantGroup{},
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

func (r *Registry) UpsertChannel(ch *types.Channel, upstreamKey string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channels[ch.Name] = ChannelEntry{Channel: ch, UpstreamKey: upstreamKey}
}

func (r *Registry) EvictChannel(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.channels, name)
}

func (r *Registry) Channel(name string) (ChannelEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.channels[name]
	return e, ok
}

func (r *Registry) Channels() []ChannelEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ChannelEntry, 0, len(r.channels))
	for _, v := range r.channels {
		out = append(out, v)
	}
	return out
}

func (r *Registry) UpsertGroup(g *types.TenantGroup) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.groups[g.Name] = g
}

func (r *Registry) EvictGroup(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.groups, name)
}

func (r *Registry) Group(name string) (*types.TenantGroup, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.groups[name]
	return g, ok
}

func (r *Registry) Groups() []*types.TenantGroup {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*types.TenantGroup, 0, len(r.groups))
	for _, v := range r.groups {
		out = append(out, v)
	}
	return out
}

// GroupChannels returns the group's usable channels, in channelRefs order.
func (r *Registry) GroupChannels(group string) []ChannelEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.groups[group]
	if !ok {
		return nil
	}
	var out []ChannelEntry
	for _, name := range g.Spec.ChannelRefs {
		if e, ok := r.channels[name]; ok && e.Usable() {
			out = append(out, e)
		}
	}
	return out
}

func (r *Registry) Stats() map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return map[string]int{"routes": len(r.routes), "channels": len(r.channels), "groups": len(r.groups)}
}
