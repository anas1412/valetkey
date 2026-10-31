package reconciler

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// recheckRejected is how often a rejected key is tried again, in case the provider's
// refusal was temporary or the key was re-enabled on the provider's side.
const recheckRejected = 10 * time.Minute

// checkKey asks the provider whether it accepts the provider's key and records the verdict.
func (c *Controller) checkKey(name string) {
	e, ok := c.registry.Provider(name)
	if !ok || e.UpstreamKey == "" || c.CheckKey == nil {
		return
	}
	c.KeyVerdict(name, c.CheckKey(context.Background(), e.Provider, e.UpstreamKey))
}

// KeyVerdict records whether the provider accepts the provider's key: rejected is why it
// refused (HTTP 401/403), "" if it didn't. A rejected provider gets no traffic. Called
// after a key check, a Test from the dashboard, or a request the provider answered 401.
func (c *Controller) KeyVerdict(name, rejected string) {
	if rejected != "" {
		c.scheduleRecheck(name)
	}
	if !c.registry.SetRejected(name, rejected) {
		return
	}
	if rejected != "" {
		slog.Warn("provider rejected the provider key", "provider", name, "reason", rejected)
		c.stats.Event("warn", "provider-key", fmt.Sprintf("provider %s: %s, no traffic is sent to it", name, rejected))
	} else {
		c.stats.Event("info", "provider-key", fmt.Sprintf("provider %s: the provider accepts the key again", name))
	}
	if e, ok := c.registry.Provider(name); ok {
		c.resolveProvider(e.Provider)
	}
}

func (c *Controller) scheduleRecheck(name string) {
	c.chanMu.Lock()
	defer c.chanMu.Unlock()
	if c.rechecks[name] {
		return
	}
	c.rechecks[name] = true
	time.AfterFunc(recheckRejected, func() {
		c.chanMu.Lock()
		delete(c.rechecks, name)
		c.chanMu.Unlock()
		if e, ok := c.registry.Provider(name); ok && e.Rejected != "" {
			c.checkKey(name)
		}
	})
}
