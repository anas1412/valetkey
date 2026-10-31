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

// Scope is what a budget belongs to: a key ("key", namespace, name), one model on
// a key ("key-model", namespace, name, model) or a tenant ("tenant", "", name).
type Scope struct {
	Kind, Namespace, Name, Model string
}

func KeyScope(ns, name string) Scope { return Scope{Kind: "key", Namespace: ns, Name: name} }
func KeyModelScope(ns, name, model string) Scope {
	return Scope{Kind: "key-model", Namespace: ns, Name: name, Model: model}
}
func TenantScope(name string) Scope { return Scope{Kind: "tenant", Name: name} }

func (s Scope) String() string {
	switch s.Kind {
	case "key":
		return "key " + s.Namespace + "/" + s.Name
	case "key-model":
		return "model " + s.Model + " on key " + s.Namespace + "/" + s.Name
	}
	return "tenant " + s.Name
}

// Item is one scope a request counts against, with its budget (nil = none).
type Item struct {
	Scope Scope
	Limit *Limit
}

// state is one scope's period. spent, requests and unpriced are this pod's own;
// others are the shares other replicas saved in status.
type state struct {
	start              time.Time
	spent, reserved    pricing.USD
	requests, unpriced int64
	others             pricing.USD
	othersRequests     int64
	stale              []string // other pods' shares from an old period, to remove
	observed, dirty    bool
	alerted            int // highest alert level sent this period (80, 100)
}

func (st *state) total() pricing.USD { return st.spent + st.others }

// Alert levels: a budget is 80% used, or used up (a request was refused or spend
// reached the budget).
const (
	AlmostUsed = 80
	Reached    = 100
)

// Alert is sent once per level per period.
type Alert struct {
	Scope  Scope
	Level  int
	Spent  pricing.USD
	Limit  Limit
	Resets time.Time
}

// Tracker holds spend for every key, key model and tenant, and reserves budget.
type Tracker struct {
	mu     sync.Mutex
	states map[Scope]*state
	now    func() time.Time
	self   string // this pod's name, the key of its share in status
	alerts chan Alert
}

// Alerts delivers budget alerts; read it or they are dropped.
func (t *Tracker) Alerts() <-chan Alert { return t.alerts }

// alert records a crossed level (caller holds the lock).
func (t *Tracker) alert(s Scope, st *state, l *Limit, level int, now time.Time) {
	if l == nil || st.alerted >= level {
		return
	}
	st.alerted = level
	select {
	case t.alerts <- Alert{Scope: s, Level: level, Spent: st.total(), Limit: *l, Resets: l.End(now)}:
	default: // nobody listening fast enough; the metrics still show it
	}
}

// levelOf is the alert level spend has reached.
func levelOf(spent pricing.USD, l *Limit) int {
	switch {
	case l == nil || l.Amount == 0:
		return 0
	case spent >= l.Amount:
		return Reached
	case spent*100 >= l.Amount*AlmostUsed:
		return AlmostUsed
	}
	return 0
}

// NewTracker tracks spend for the pod named self.
func NewTracker(self string) *Tracker {
	return &Tracker{states: map[Scope]*state{}, now: time.Now, self: self, alerts: make(chan Alert, 64)}
}

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
		*st = state{start: start, reserved: st.reserved, observed: st.observed, dirty: true} // alerts reset too
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
		if it.Limit != nil && st.total()+st.reserved+amount > it.Limit.Amount {
			t.alert(it.Scope, st, it.Limit, Reached, now)
			return nil, &ExceededError{Scope: it.Scope, Limit: *it.Limit, Resets: it.Limit.End(now), Spent: st.total(), Request: amount}
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
		if lvl := levelOf(cur.total(), it.Limit); lvl > 0 {
			t.alert(it.Scope, cur, it.Limit, lvl, now)
		}
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
	return Usage{Spent: st.total(), Reserved: st.reserved, Requests: st.requests + st.othersRequests, Unpriced: st.unpriced, PeriodStart: st.start}
}

// Observe reads the spend saved in a CRD's status: other replicas' shares of the
// current period are added to this pod's own, and this pod's own share is loaded
// once, after a restart (a restarted container keeps its pod name).
func (t *Tracker) Observe(s Scope, l *Limit, saved *types.Spend) {
	t.mu.Lock()
	defer t.mu.Unlock()
	st := t.current(s, l, t.now())
	st.others, st.othersRequests, st.stale = 0, 0, nil
	if saved != nil {
		for pod, sh := range saved.Pods {
			if sh == nil {
				continue
			}
			if !samePeriod(sh.PeriodStart, st.start) {
				if pod != t.self {
					st.stale = append(st.stale, pod)
				}
				continue
			}
			usd, err := pricing.ParseUSD(sh.USD)
			if err != nil {
				continue
			}
			if pod == t.self {
				if !st.observed {
					st.spent, st.requests, st.unpriced = usd, sh.Requests, sh.Unpriced
				}
				continue
			}
			st.others += usd
			st.othersRequests += sh.Requests
		}
	}
	st.observed = true
	// Levels already reached before this pod saw them were alerted by whoever got there.
	st.alerted = max(st.alerted, levelOf(st.total(), l))
}

func samePeriod(saved string, start time.Time) bool {
	t, err := time.Parse(time.RFC3339, saved)
	return err == nil && t.Equal(start)
}

// Forget drops a deleted key or tenant.
func (t *Tracker) Forget(s Scope) {
	t.mu.Lock()
	delete(t.states, s)
	t.mu.Unlock()
}

// Saver writes a scope's spend to its CRD status (merge patch: other pods' shares
// are kept, nil shares are removed).
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
		if !st.dirty {
			continue
		}
		st.dirty = false
		start := st.start.UTC().Format(time.RFC3339)
		pods := map[string]*types.PodSpend{t.self: {PeriodStart: start, USD: st.spent.Decimal(), Requests: st.requests, Unpriced: st.unpriced}}
		for _, pod := range st.stale {
			pods[pod] = nil
		}
		st.stale = nil
		pending[s] = types.Spend{PeriodStart: start, USD: st.total().Decimal(), Requests: st.requests + st.othersRequests,
			Unpriced: st.unpriced, UpdatedAt: now, Pods: pods}
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
