package reconciler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/anas1412/llmp/internal/keys"
	"github.com/anas1412/llmp/internal/registry"
	"github.com/anas1412/llmp/internal/status"
	"github.com/anas1412/llmp/internal/types"
)

// SecretName is the tenant Secret holding a ProxyKey's virtual key.
func SecretName(proxyKey string) string { return proxyKey + "-llmproxy" }

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

	// Mint a virtual key unless the Secret already holds the one recorded in status.
	secretName := SecretName(name)
	keyHash := pk.Status.KeyHash
	secret, err := c.kube.ReadSecret(ctx, ns, secretName)
	if err != nil {
		return err
	}
	secretHash := ""
	if secret != nil && secret.Data["LLMPROXY_KEY"] != "" {
		secretHash = keys.Hash(secret.Data["LLMPROXY_KEY"])
	}
	if secretHash == "" || (keyHash != "" && secretHash != keyHash) {
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
		Models: models, AllowList: allow, RPM: pk.Spec.RPM, TPM: pk.Spec.TPM,
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
	return c.kube.ApplySecret(ctx, pk.Namespace, secretName, map[string]string{"app.kubernetes.io/managed-by": "llmproxy-operator"}, owner,
		map[string]string{
			"LLMPROXY_KEY":      key,
			"LLMPROXY_ENDPOINT": c.cfg.ProxyURL,
			"LLMPROXY_CHANNEL":  channel,
			"LLMPROXY_GROUP":    group,
		})
}

// setStatus patches only when something changed, so status writes don't loop through the informer.
func (c *Controller) setStatus(ctx context.Context, pk *types.ProxyKey, next types.Status) error {
	if !status.Changed(pk.Status, next) {
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
