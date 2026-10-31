package reconciler

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/anas1412/valetkey/internal/budget"
	"log/slog"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/anas1412/valetkey/internal/status"
	"github.com/anas1412/valetkey/internal/types"
)

func (c *Controller) onProvider(obj any) {
	if ch, ok := types.FromUnstructured[types.Provider](obj); ok {
		c.resolveProvider(ch)
	}
}

// resolveProvider reads the provider's upstream key from the provider Secret and updates its status.
// Called from both the provider and secret informers, so it holds chanMu.
func (c *Controller) resolveProvider(ch *types.Provider) {
	c.chanMu.Lock()
	defer c.chanMu.Unlock()
	key, problem := c.providerKey(ch.Spec.KeySecretRef)
	old, known := c.registry.Provider(ch.Name)
	c.registry.UpsertProvider(ch, key)
	// A new key (or provider) gets checked once; the verdict lasts until it changes.
	if key != "" && (!known || !old.SameKey(ch, key)) {
		go c.checkKey(ch.Name)
	}

	if prev, seen := c.problems[ch.Name]; !seen || prev != problem {
		switch {
		case problem != "":
			slog.Warn("provider key problem", "provider", ch.Name, "problem", problem)
			c.stats.Event("warn", "provider-key", fmt.Sprintf("provider %s: %s", ch.Name, problem))
		case seen:
			c.stats.Event("info", "provider-key", fmt.Sprintf("provider %s: upstream key resolved", ch.Name))
		}
		c.problems[ch.Name] = problem
	}

	reason, msg := "KeyResolved", "upstream key resolved"
	if problem != "" {
		reason, msg = "SecretUnavailable", problem
	} else if e, _ := c.registry.Provider(ch.Name); e.Rejected != "" {
		reason, msg = "KeyRejected", e.Rejected
	}
	next := status.Ready(ch.Status, reason == "KeyResolved", reason, msg, ch.Generation)
	if status.Changed(ch.Status, next) && c.leading() {
		if err := c.kube.PatchStatus(context.Background(), types.ProviderGVR, c.cfg.OperatorNamespace, ch.Name, next); err != nil {
			slog.Warn("provider status patch failed", "provider", ch.Name, "err", err)
		}
	}
	c.providersChanged()
}

func (c *Controller) onProviderDelete(obj any) {
	if u, ok := obj.(*unstructured.Unstructured); ok {
		c.chanMu.Lock()
		defer c.chanMu.Unlock()
		c.registry.EvictProvider(u.GetName())
		delete(c.problems, u.GetName())
		c.providersChanged()
	}
}

// onSecret re-resolves every provider that references the changed (or deleted) Secret.
func (c *Controller) onSecret(obj any) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	for _, e := range c.registry.Providers() {
		if e.Provider.Spec.KeySecretRef.Name == u.GetName() {
			c.resolveProvider(e.Provider)
		}
	}
}

// providerKey reads a key from a Secret in the operator namespace via the informer cache.
func (c *Controller) providerKey(ref types.SecretRef) (key, problem string) {
	o, err := c.secrets.ByNamespace(c.cfg.OperatorNamespace).Get(ref.Name)
	if err != nil {
		return "", fmt.Sprintf("secret %q not found", ref.Name)
	}
	data, _, _ := unstructured.NestedStringMap(o.(*unstructured.Unstructured).Object, "data")
	b, err := base64.StdEncoding.DecodeString(data[ref.KeyName()])
	if err != nil || len(b) == 0 {
		return "", fmt.Sprintf("key %q missing in secret %q", ref.KeyName(), ref.Name)
	}
	return strings.TrimSpace(string(b)), ""
}

func (c *Controller) providersChanged() {
	c.metrics.ProvidersActive.Reset()
	counts := map[string]int{}
	for _, e := range c.registry.Providers() {
		if e.Usable() {
			counts[e.Provider.Spec.Type]++
		}
	}
	for t, n := range counts {
		c.metrics.ProvidersActive.WithLabelValues(t).Set(float64(n))
	}
	for _, g := range c.registry.Tenants() {
		c.refreshTenantStatus(g)
	}
	c.RequeueAll()
}

func (c *Controller) onTenant(obj any) {
	g, ok := types.FromUnstructured[types.Tenant](obj)
	if !ok {
		return
	}
	c.registry.UpsertTenant(g)
	if c.budgets != nil {
		if limit, err := budget.Parse(g.Spec.Budget); err == nil {
			c.budgets.Observe(budget.TenantScope(g.Name), limit, g.Status.Spend)
		}
	}
	c.refreshTenantStatus(g)
	c.RequeueAll()
}

func (c *Controller) onTenantDelete(obj any) {
	if u, ok := obj.(*unstructured.Unstructured); ok {
		c.registry.EvictTenant(u.GetName())
		c.RequeueAll()
	}
}

// refreshTenantStatus: Ready = enabled and at least one referenced provider is usable.
func (c *Controller) refreshTenantStatus(g *types.Tenant) {
	usable := len(c.registry.TenantProviders(g.Name))
	var missing []string
	for _, ref := range g.Spec.Providers {
		if _, ok := c.registry.Provider(ref); !ok {
			missing = append(missing, ref)
		}
	}
	reason, msg := "Ready", fmt.Sprintf("%d usable provider(s)", usable)
	_, budgetErr := budget.Parse(g.Spec.Budget)
	switch {
	case budgetErr != nil:
		reason, msg = "InvalidBudget", budgetErr.Error()
	case !g.Enabled():
		reason, msg = "Disabled", "tenant is disabled"
	case usable == 0:
		reason, msg = "NoProviders", "no referenced provider is enabled with a resolved key"
	}
	if len(missing) > 0 {
		msg += "; unknown providers: " + strings.Join(missing, ", ")
	}
	next := status.Ready(g.Status, g.Enabled() && usable > 0 && budgetErr == nil, reason, msg, g.Generation)
	if !c.leading() || !status.Changed(g.Status, next) {
		return
	}
	if err := c.kube.PatchStatus(context.Background(), types.TenantGVR, c.cfg.OperatorNamespace, g.Name, next); err != nil {
		slog.Warn("tenant status patch failed", "tenant", g.Name, "err", err)
	}
}
