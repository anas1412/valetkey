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

func (c *Controller) onChannel(obj any) {
	if ch, ok := types.FromUnstructured[types.Channel](obj); ok {
		c.resolveChannel(ch)
	}
}

// resolveChannel reads the channel's upstream key from the provider Secret and updates its status.
// Called from both the channel and secret informers, so it holds chanMu.
func (c *Controller) resolveChannel(ch *types.Channel) {
	c.chanMu.Lock()
	defer c.chanMu.Unlock()
	key, problem := c.providerKey(ch.Spec.KeySecretRef)
	c.registry.UpsertChannel(ch, key)

	if prev, seen := c.problems[ch.Name]; !seen || prev != problem {
		switch {
		case problem != "":
			slog.Warn("channel key problem", "channel", ch.Name, "problem", problem)
			c.stats.Event("warn", "channel-key", fmt.Sprintf("channel %s: %s", ch.Name, problem))
		case seen:
			c.stats.Event("info", "channel-key", fmt.Sprintf("channel %s: upstream key resolved", ch.Name))
		}
		c.problems[ch.Name] = problem
	}

	reason, msg := "KeyResolved", "upstream key resolved"
	if problem != "" {
		reason, msg = "SecretUnavailable", problem
	}
	next := status.Ready(ch.Status, problem == "", reason, msg, ch.Generation)
	if status.Changed(ch.Status, next) {
		if err := c.kube.PatchStatus(context.Background(), types.ChannelGVR, c.cfg.OperatorNamespace, ch.Name, next); err != nil {
			slog.Warn("channel status patch failed", "channel", ch.Name, "err", err)
		}
	}
	c.channelsChanged()
}

func (c *Controller) onChannelDelete(obj any) {
	if u, ok := obj.(*unstructured.Unstructured); ok {
		c.chanMu.Lock()
		defer c.chanMu.Unlock()
		c.registry.EvictChannel(u.GetName())
		delete(c.problems, u.GetName())
		c.channelsChanged()
	}
}

// onSecret re-resolves every channel that references the changed (or deleted) Secret.
func (c *Controller) onSecret(obj any) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	for _, e := range c.registry.Channels() {
		if e.Channel.Spec.KeySecretRef.Name == u.GetName() {
			c.resolveChannel(e.Channel)
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

func (c *Controller) channelsChanged() {
	c.metrics.ChannelsActive.Reset()
	counts := map[string]int{}
	for _, e := range c.registry.Channels() {
		if e.Usable() {
			counts[e.Channel.Spec.Type]++
		}
	}
	for t, n := range counts {
		c.metrics.ChannelsActive.WithLabelValues(t).Set(float64(n))
	}
	for _, g := range c.registry.Groups() {
		c.refreshGroupStatus(g)
	}
	c.RequeueAll()
}

func (c *Controller) onGroup(obj any) {
	g, ok := types.FromUnstructured[types.TenantGroup](obj)
	if !ok {
		return
	}
	c.registry.UpsertGroup(g)
	if c.budgets != nil {
		if limit, err := budget.Parse(g.Spec.Budget); err == nil {
			c.budgets.Restore(budget.TenantScope(g.Name), limit, g.Status.Spend)
		}
	}
	c.refreshGroupStatus(g)
	c.RequeueAll()
}

func (c *Controller) onGroupDelete(obj any) {
	if u, ok := obj.(*unstructured.Unstructured); ok {
		c.registry.EvictGroup(u.GetName())
		c.RequeueAll()
	}
}

// refreshGroupStatus: Ready = enabled and at least one referenced channel is usable.
func (c *Controller) refreshGroupStatus(g *types.TenantGroup) {
	usable := len(c.registry.GroupChannels(g.Name))
	var missing []string
	for _, ref := range g.Spec.ChannelRefs {
		if _, ok := c.registry.Channel(ref); !ok {
			missing = append(missing, ref)
		}
	}
	reason, msg := "Ready", fmt.Sprintf("%d usable channel(s)", usable)
	_, budgetErr := budget.Parse(g.Spec.Budget)
	switch {
	case budgetErr != nil:
		reason, msg = "InvalidBudget", budgetErr.Error()
	case !g.Enabled():
		reason, msg = "Disabled", "group is disabled"
	case usable == 0:
		reason, msg = "NoChannels", "no referenced channel is enabled with a resolved key"
	}
	if len(missing) > 0 {
		msg += "; unknown channels: " + strings.Join(missing, ", ")
	}
	next := status.Ready(g.Status, g.Enabled() && usable > 0 && budgetErr == nil, reason, msg, g.Generation)
	if !status.Changed(g.Status, next) {
		return
	}
	if err := c.kube.PatchStatus(context.Background(), types.GroupGVR, c.cfg.OperatorNamespace, g.Name, next); err != nil {
		slog.Warn("group status patch failed", "group", g.Name, "err", err)
	}
}
