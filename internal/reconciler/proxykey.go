package reconciler

import (
	"context"
	"fmt"
	"github.com/anas1412/valetkey/internal/budget"
	"log/slog"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/anas1412/valetkey/internal/keys"
	"github.com/anas1412/valetkey/internal/registry"
	"github.com/anas1412/valetkey/internal/status"
	"github.com/anas1412/valetkey/internal/types"
)

// SecretName is the tenant Secret holding a ProxyKey's virtual key.
func SecretName(proxyKey string) string { return proxyKey + "-valetkey" }

func (c *Controller) reconcile(ctx context.Context, id string) error {
	ns, name, _ := strings.Cut(id, "/")
	obj, err := c.proxyKeys.ByNamespace(ns).Get(name)
	if apierrors.IsNotFound(err) {
		return nil // deleted; onProxyKeyDelete evicted the route
	}
	if err != nil {
		return err
	}
	pk, ok := types.FromUnstructured[types.ProxyKey](obj)
	if !ok {
		return fmt.Errorf("cannot decode ProxyKey")
	}
	uid := string(pk.UID)
	// The group is always the ProxyKey's namespace; tenants can't pick another one.
	group := ns

	notReady := func(reason, msg string) error {
		c.registry.EvictRoute(uid)
		c.refreshKeyGauge()
		return c.setStatus(ctx, pk, status.Ready(pk.Status, false, reason, msg, pk.Generation))
	}

	if !pk.Enabled() {
		return notReady("Disabled", "key is disabled")
	}
	g, ok := c.registry.Group(group)
	if !ok {
		return notReady("GroupNotFound", fmt.Sprintf("group %q not found", group))
	}
	if !g.Enabled() {
		return notReady("GroupDisabled", fmt.Sprintf("group %q is disabled", group))
	}
	channels := c.registry.GroupChannels(group)
	if len(channels) == 0 {
		return notReady("NoChannels", fmt.Sprintf("group %q has no enabled channels with a resolved key", group))
	}
	var allow keys.AllowList
	if len(pk.Spec.Subnets) > 0 {
		if allow, err = keys.ParseAllowList(pk.Spec.Subnets); err != nil {
			return notReady("InvalidSubnets", err.Error())
		}
	}

	var expires time.Time
	if pk.Spec.ExpiresAt != "" {
		if expires, err = time.Parse(time.RFC3339, pk.Spec.ExpiresAt); err != nil {
			return notReady("InvalidExpiry", "expiresAt must be an RFC 3339 time, e.g. 2026-12-31T23:59:59Z")
		}
		if !time.Now().Before(expires) {
			return notReady("Expired", "key expired at "+expires.UTC().Format(time.RFC3339))
		}
		c.queue.AddAfter(id, time.Until(expires)+time.Second) // re-check right after it expires
	}
	limit, err := budget.Parse(pk.Spec.Budget)
	if err != nil {
		return notReady("InvalidBudget", err.Error())
	}
	modelLimits := map[string]*budget.Limit{}
	for model, b := range pk.Spec.ModelBudgets {
		l, err := budget.Parse(&b)
		if err != nil {
			return notReady("InvalidBudget", fmt.Sprintf("budget for model %s: %v", model, err))
		}
		modelLimits[model] = l
	}
	c.observeKeySpend(pk, limit, modelLimits)

	// Mint a virtual key unless the Secret already holds the one recorded in status.
	secretName := SecretName(name)
	keyHash := pk.Status.KeyHash
	secret, err := c.kube.ReadSecret(ctx, ns, secretName)
	if err != nil {
		return err
	}
	secretHash := ""
	if secret != nil && secret.Data["VALETKEY_KEY"] != "" {
		secretHash = keys.Hash(secret.Data["VALETKEY_KEY"])
	}
	needsKey := secretHash == "" || (keyHash != "" && secretHash != keyHash)
	if needsKey && !c.leading() {
		// The leader mints it; look again shortly.
		c.queue.AddAfter(id, 3*time.Second)
		return nil
	}
	if needsKey {
		virtual := keys.Generate()
		keyHash = keys.Hash(virtual)
		if err := c.writeTenantSecret(ctx, pk, secretName, virtual, channels[0].Name(), group); err != nil {
			return err
		}
		slog.Info("minted key", "proxykey", id, "secret", secretName)
		c.stats.Event("info", "key-minted", fmt.Sprintf("new key for %s (secret %s)", id, secretName))
		c.metrics.KeysMinted.WithLabelValues(group).Inc()
	} else if keyHash == "" {
		keyHash = secretHash
	}

	var models []string
	if len(pk.Spec.Models) > 0 {
		models = pk.Spec.Models
	}
	c.registry.UpsertRoute(uid, registry.Route{
		KeyHash: keyHash, Group: group, Namespace: ns, ProxyKeyName: name,
		Models: models, AllowList: allow, RPM: pk.Spec.RPM, TPM: pk.Spec.TPM, Budget: limit, ModelBudgets: modelLimits,
		MaxTokens: pk.Spec.MaxTokens, ExpiresAt: expires,
	})
	c.refreshKeyGauge()

	next := status.Ready(pk.Status, true, "Active", "Active", pk.Generation)
	next.KeyHash, next.SecretName = keyHash, secretName
	return c.setStatus(ctx, pk, next)
}

func (c *Controller) writeTenantSecret(ctx context.Context, pk *types.ProxyKey, secretName, key, channel, group string) error {
	f := false
	t := true
	owner := &metav1.OwnerReference{
		APIVersion: types.Group + "/" + types.Version, Kind: "ProxyKey",
		Name: pk.Name, UID: pk.UID, Controller: &t, BlockOwnerDeletion: &f,
	}
	return c.kube.ApplySecret(ctx, pk.Namespace, secretName, map[string]string{"app.kubernetes.io/managed-by": "valetkey"}, owner,
		map[string]string{
			"VALETKEY_KEY":      key,
			"VALETKEY_ENDPOINT": c.cfg.ProxyURL,
			"VALETKEY_CHANNEL":  channel,
			"VALETKEY_GROUP":    group,
		})
}

// leading reports whether this pod does the writes (one replica does).
func (c *Controller) leading() bool { return c.IsLeader == nil || c.IsLeader() }

// setStatus patches only when something changed, so status writes don't loop through
// the informer. Only the leader writes.
func (c *Controller) setStatus(ctx context.Context, pk *types.ProxyKey, next types.Status) error {
	if !c.leading() || !status.Changed(pk.Status, next) {
		return nil
	}
	return c.kube.PatchStatus(ctx, types.ProxyKeyGVR, pk.Namespace, pk.Name, next)
}

func (c *Controller) onProxyKeyDelete(obj any) {
	if u, ok := obj.(*unstructured.Unstructured); ok {
		c.registry.EvictRoute(string(u.GetUID()))
		c.refreshKeyGauge()
	}
}

func (c *Controller) refreshKeyGauge() {
	counts := map[string]int{}
	for _, g := range c.registry.Groups() {
		counts[g.Name] = 0
	}
	for _, r := range c.registry.Routes() {
		counts[r.Group]++
	}
	c.metrics.ProxyKeysActive.Reset()
	for g, n := range counts {
		c.metrics.ProxyKeysActive.WithLabelValues(g).Set(float64(n))
	}
}

func everything() labels.Selector { return labels.Everything() }

// observeKeySpend hands the spend saved in a key's status to the budget tracker.
func (c *Controller) observeKeySpend(pk *types.ProxyKey, limit *budget.Limit, models map[string]*budget.Limit) {
	if c.budgets == nil {
		return
	}
	c.budgets.Observe(budget.KeyScope(pk.Namespace, pk.Name), limit, pk.Status.Spend)
	for model, l := range models {
		c.budgets.Observe(budget.KeyModelScope(pk.Namespace, pk.Name, model), l, pk.Status.ModelSpend[model])
	}
}

// onSpendUpdate reads another replica's saved spend without reconciling.
func (c *Controller) onSpendUpdate(obj any) {
	if c.budgets == nil {
		return
	}
	if pk, ok := types.FromUnstructured[types.ProxyKey](obj); ok && pk.Kind == "ProxyKey" {
		models := map[string]*budget.Limit{}
		for m, b := range pk.Spec.ModelBudgets {
			models[m] = budget.ParseCached(&b)
		}
		c.observeKeySpend(pk, budget.ParseCached(pk.Spec.Budget), models)
		return
	}
	if g, ok := types.FromUnstructured[types.TenantGroup](obj); ok && g.Kind == "Group" {
		c.budgets.Observe(budget.TenantScope(g.Name), budget.ParseCached(g.Spec.Budget), g.Status.Spend)
	}
}
