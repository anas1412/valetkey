package budget

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/anas1412/valetkey/internal/pricing"
	"github.com/anas1412/valetkey/internal/types"
)

// Scope is what a budget belongs to: a key ("key", namespace, name) or a tenant
// ("tenant", "", group name).
type Scope struct {
	Kind, Namespace, Name string
}

func KeyScope(ns, name string) Scope { return Scope{Kind: "key", Namespace: ns, Name: name} }
func TenantScope(name string) Scope  { return Scope{Kind: "tenant", Name: name} }

func (s Scope) String() string {
	if s.Kind == "key" {
		return "key " + s.Namespace + "/" + s.Name
	}
	return "tenant " + s.Name
}

// Item is one scope a request counts against, with its budget (nil = none).
type Item struct {
	Scope Scope
	Limit *Limit
}

type state struct {
	start              time.Time
	spent, reserved    pricing.USD
	requests, unpriced int64
	dirty              bool
}

// Tracker holds spend for every key and tenant in memory and reserves budget.
type Tracker struct {
	mu     sync.Mutex
	states map[Scope]*state
	now    func() time.Time
}

func NewTracker() *Tracker { return &Tracker{states: map[Scope]*state{}, now: time.Now} }

// current returns the scope's state for the period containing now, starting a
// new period (spend 0) when the old one is over.
func (t *Tracker) current(s Scope, l *Limit, now time.Time) *state {
	period := untracked
	if l != nil {
		period = *l
	}
	start := period.Start(now)
	st := t.states[s]
	if st == nil {
		st = &state{start: start}
		t.states[s] = st
	} else if !st.start.Equal(start) {
		st.start, st.spent, st.requests, st.unpriced, st.dirty = start, 0, 0, 0, true
	}
	return st
}

// ExceededError says which budget a request didn't fit in.
type ExceededError struct {
	Scope   Scope
	Limit   Limit
	Resets  time.Time
	Spent   pricing.USD
	Request pricing.USD
}

func (e *ExceededError) Error() string {
	return fmt.Sprintf("%s budget of %s for %s reached (spent %s, this request may cost up to %s); resets %s",
		titleCase(e.Limit.Period), e.Limit.Amount, e.Scope, e.Spent, e.Request, e.Resets.UTC().Format("2006-01-02 15:04 UTC"))
}

func titleCase(s string) string { return strings.ToUpper(s[:1]) + s[1:] }

// Reservation is budget held for one request until it is committed or released.
type Reservation struct {
	t      *Tracker
	items  []Item
	amount pricing.USD
	done   bool
}

// Reserve holds amount on every item. It fails, holding nothing, if any budget
// can't fit spend + already reserved + amount.
func (t *Tracker) Reserve(items []Item, amount pricing.USD) (*Reservation, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	for _, it := range items {
		st := t.current(it.Scope, it.Limit, now)
		if it.Limit != nil && st.spent+st.reserved+amount > it.Limit.Amount {
			return nil, &ExceededError{Scope: it.Scope, Limit: *it.Limit, Resets: it.Limit.End(now), Spent: st.spent, Request: amount}
		}
	}
	for _, it := range items {
		t.current(it.Scope, it.Limit, now).reserved += amount
	}
	return &Reservation{t: t, items: items, amount: amount}, nil
}

// Commit releases the hold and charges the real cost. unpriced marks requests
// that couldn't be priced (only possible without a budget).
func (r *Reservation) Commit(cost pricing.USD, unpriced bool) {
	r.finish(func(st *state) {
		st.spent += cost
		st.requests++
		if unpriced {
			st.unpriced++
		}
	})
}

// Release drops the hold without charging (the request never reached a provider).
func (r *Reservation) Release() { r.finish(func(*state) {}) }

func (r *Reservation) finish(charge func(*state)) {
	t := r.t
	t.mu.Lock()
	defer t.mu.Unlock()
	if r.done {
		return
	}
	r.done = true
	now := t.now()
	for _, it := range r.items {
		st := t.states[it.Scope]
		if st == nil {
			continue
		}
		st.reserved = max(st.reserved-r.amount, 0)
		cur := t.current(it.Scope, it.Limit, now)
		charge(cur)
		cur.dirty = true
	}
}

// Usage is a scope's spend in the current period.
type Usage struct {
	Spent, Reserved pricing.USD
	Requests        int64
	Unpriced        int64
	PeriodStart     time.Time
}

// Usage reports a scope's current-period spend.
func (t *Tracker) Usage(s Scope, l *Limit) Usage {
	t.mu.Lock()
	defer t.mu.Unlock()
	st := t.current(s, l, t.now())
	return Usage{Spent: st.spent, Reserved: st.reserved, Requests: st.requests, Unpriced: st.unpriced, PeriodStart: st.start}
}

// Restore loads spend saved in a CRD's status. It only applies when the scope has
// no spend in memory yet and the saved period is the current one.
func (t *Tracker) Restore(s Scope, l *Limit, saved *types.Spend) {
	if saved == nil {
		return
	}
	start, err := time.Parse(time.RFC3339, saved.PeriodStart)
	if err != nil {
		return
	}
	spent, err := pricing.ParseUSD(saved.USD)
	if err != nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.states[s]; ok {
		return
	}
	st := t.current(s, l, t.now())
	if st.start.Equal(start) {
		st.spent, st.requests, st.unpriced = spent, saved.Requests, saved.Unpriced
	}
}

// Forget drops a deleted key or tenant.
func (t *Tracker) Forget(s Scope) {
	t.mu.Lock()
	delete(t.states, s)
	t.mu.Unlock()
}

// Saver writes a scope's spend to its CRD status.
type Saver func(ctx context.Context, s Scope, spend types.Spend) error

// Run saves changed spend every interval, and once more when ctx ends, so a
// restart loses at most one interval of spend.
func (t *Tracker) Run(ctx context.Context, interval time.Duration, save Saver) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			t.flush(ctx, save)
		case <-ctx.Done():
			final, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			t.flush(final, save)
			cancel()
			return
		}
	}
}

func (t *Tracker) flush(ctx context.Context, save Saver) {
	t.mu.Lock()
	now := t.now().UTC().Format(time.RFC3339)
	pending := map[Scope]types.Spend{}
	for s, st := range t.states {
		if st.dirty {
			st.dirty = false
			pending[s] = types.Spend{PeriodStart: st.start.Format(time.RFC3339), USD: st.spent.Decimal(),
				Requests: st.requests, Unpriced: st.unpriced, UpdatedAt: now}
		}
	}
	t.mu.Unlock()
	for s, sp := range pending {
		if err := save(ctx, s, sp); err != nil {
			slog.Warn("could not save spend", "scope", s.String(), "err", err)
			t.mu.Lock()
			if st := t.states[s]; st != nil {
				st.dirty = true // retry next time
			}
			t.mu.Unlock()
		}
	}
}
