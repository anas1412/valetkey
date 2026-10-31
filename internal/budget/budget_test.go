package budget

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/anas1412/valetkey/internal/pricing"
	"github.com/anas1412/valetkey/internal/types"
)

func limit(t *testing.T, usd, period, tz string) *Limit {
	t.Helper()
	l, err := Parse(&types.Budget{USD: usd, Period: period, Timezone: tz})
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func cents(n int64) pricing.USD { return pricing.USD(n) * pricing.Dollar / 100 }

// Many requests racing for the last dollars never push spend past the budget,
// even when each one ends up costing its full reservation.
func TestReservationsNeverExceedBudget(t *testing.T) {
	tr := NewTracker("pod-a")
	key, tenant := KeyScope("billing", "api"), TenantScope("billing")
	items := []Item{{key, limit(t, "1", "", "")}, {tenant, limit(t, "5", "", "")}}

	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted := 0
	for range 200 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := tr.Reserve(items, cents(3))
			if err != nil {
				var ex *ExceededError
				if !errors.As(err, &ex) {
					t.Error(err)
				}
				return
			}
			mu.Lock()
			accepted++
			mu.Unlock()
			r.Commit(cents(3), false)
		}()
	}
	wg.Wait()
	u := tr.Usage(key, items[0].Limit)
	if u.Spent > pricing.Dollar || accepted != 33 || u.Reserved != 0 {
		t.Fatalf("spent %s, accepted %d, reserved %s", u.Spent, accepted, u.Reserved)
	}
}

func TestCommitChargesActualCost(t *testing.T) {
	tr := NewTracker("pod-a")
	key := KeyScope("ns", "k")
	l := limit(t, "1", "daily", "")
	r, err := tr.Reserve([]Item{{key, l}}, cents(90))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Reserve([]Item{{key, l}}, cents(20)); err == nil {
		t.Fatal("second reservation should not fit while the first is held")
	}
	r.Commit(cents(10), false)
	r.Commit(cents(10), false) // no double charge
	if u := tr.Usage(key, l); u.Spent != cents(10) || u.Requests != 1 {
		t.Fatalf("usage %+v", u)
	}
	r2, _ := tr.Reserve([]Item{{key, l}}, cents(50))
	r2.Release()
	if u := tr.Usage(key, l); u.Spent != cents(10) || u.Reserved != 0 {
		t.Fatalf("after release %+v", u)
	}
}

func TestPeriodsInTimeZones(t *testing.T) {
	tz := "Asia/Tokyo" // UTC+9
	for _, c := range []struct {
		period, at, start, end string
	}{
		{"monthly", "2026-10-31T15:30:00Z", "2026-11-01T00:00:00+09:00", "2026-12-01T00:00:00+09:00"},
		{"monthly", "2026-10-31T14:59:00Z", "2026-10-01T00:00:00+09:00", "2026-11-01T00:00:00+09:00"},
		{"daily", "2026-10-31T16:00:00Z", "2026-11-01T00:00:00+09:00", "2026-11-02T00:00:00+09:00"},
		{"weekly", "2026-11-01T10:00:00Z", "2026-10-26T00:00:00+09:00", "2026-11-02T00:00:00+09:00"}, // Sunday in Tokyo
	} {
		l := limit(t, "1", c.period, tz)
		at, _ := time.Parse(time.RFC3339, c.at)
		start, _ := time.Parse(time.RFC3339, c.start)
		end, _ := time.Parse(time.RFC3339, c.end)
		if !l.Start(at).Equal(start) || !l.End(at).Equal(end) {
			t.Errorf("%s at %s: %s – %s", c.period, c.at, l.Start(at), l.End(at))
		}
	}
	if _, err := Parse(&types.Budget{USD: "1", Period: "hourly"}); err == nil {
		t.Error("bad period accepted")
	}
}

func TestNewPeriodResetsSpend(t *testing.T) {
	tr := NewTracker("pod-a")
	now := time.Date(2026, 10, 31, 23, 0, 0, 0, time.UTC)
	tr.now = func() time.Time { return now }
	key, l := KeyScope("ns", "k"), limit(t, "1", "daily", "")
	r, _ := tr.Reserve([]Item{{key, l}}, cents(80))
	r.Commit(cents(80), false)
	if _, err := tr.Reserve([]Item{{key, l}}, cents(30)); err == nil {
		t.Fatal("should be over budget today")
	}
	now = now.Add(2 * time.Hour) // tomorrow
	if _, err := tr.Reserve([]Item{{key, l}}, cents(30)); err != nil {
		t.Fatalf("new day should reset: %v", err)
	}
}

func save(t *testing.T, tr *Tracker) map[Scope]types.Spend {
	t.Helper()
	saved := map[Scope]types.Spend{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tr.Run(ctx, time.Hour, func(_ context.Context, s Scope, sp types.Spend) error { saved[s] = sp; return nil })
	return saved
}

func TestSaveAndRestore(t *testing.T) {
	tr := NewTracker("pod-a")
	key, l := KeyScope("ns", "k"), limit(t, "10", "", "")
	tr.Observe(key, l, nil)
	r, _ := tr.Reserve([]Item{{key, l}}, cents(500))
	r.Commit(pricing.USD(1_234_567_891), false) // $0.001234567891

	sp := save(t, tr)[key]
	if sp.USD != "0.001234567891" || sp.Requests != 1 || sp.Pods["pod-a"].USD != "0.001234567891" {
		t.Fatalf("saved %+v", sp)
	}

	// The same pod after a container restart loads its own share once.
	again := NewTracker("pod-a")
	again.Observe(key, l, &sp)
	again.Observe(key, l, &types.Spend{Pods: map[string]*types.PodSpend{"pod-a": {PeriodStart: sp.PeriodStart, USD: "9"}}})
	if u := again.Usage(key, l); u.Spent != pricing.USD(1_234_567_891) || u.Requests != 1 {
		t.Fatalf("restored %+v", u)
	}
	// A replacement pod counts the old pod's share as someone else's.
	next := NewTracker("pod-b")
	next.Observe(key, l, &sp)
	if u := next.Usage(key, l); u.Spent != pricing.USD(1_234_567_891) {
		t.Fatalf("replacement pod sees %+v", u)
	}
	old := NewTracker("pod-c")
	old.Observe(key, l, &types.Spend{Pods: map[string]*types.PodSpend{"pod-x": {PeriodStart: "2020-01-01T00:00:00Z", USD: "9"}}})
	if u := old.Usage(key, l); u.Spent != 0 {
		t.Fatalf("spend from an old period should not count: %+v", u)
	}
	r2, _ := old.Reserve([]Item{{key, l}}, cents(1))
	r2.Commit(cents(1), false)
	if sp := save(t, old)[key]; sp.Pods["pod-x"] != nil || len(sp.Pods) != 2 {
		t.Fatalf("old share should be removed: %+v", sp.Pods)
	}
}

// Two replicas share one budget through the shares they save in status.
func TestReplicasShareABudget(t *testing.T) {
	key, l := TenantScope("research"), limit(t, "1", "daily", "")
	a, b := NewTracker("pod-a"), NewTracker("pod-b")
	a.Observe(key, l, nil)
	b.Observe(key, l, nil)
	r, _ := a.Reserve([]Item{{key, l}}, cents(60))
	r.Commit(cents(60), false)
	sp := save(t, a)[key]

	b.Observe(key, l, &sp)
	if _, err := b.Reserve([]Item{{key, l}}, cents(50)); err == nil {
		t.Fatal("pod-b should see pod-a's $0.60")
	}
	rb, err := b.Reserve([]Item{{key, l}}, cents(40))
	if err != nil {
		t.Fatal(err)
	}
	rb.Commit(cents(40), false)
	if got := save(t, b)[key]; got.USD != "1" || got.Pods["pod-b"].USD != "0.4" {
		t.Fatalf("pod-b saved %+v", got)
	}
}

func TestAlertsOncePerLevel(t *testing.T) {
	tr := NewTracker("pod-a")
	key, l := KeyScope("ns", "k"), limit(t, "1", "daily", "")
	for range 9 { // $0.90
		r, _ := tr.Reserve([]Item{{key, l}}, cents(10))
		r.Commit(cents(10), false)
	}
	if _, err := tr.Reserve([]Item{{key, l}}, cents(20)); err == nil {
		t.Fatal("should not fit")
	}
	var got []int
	for len(tr.Alerts()) > 0 {
		got = append(got, (<-tr.Alerts()).Level)
	}
	if len(got) != 2 || got[0] != AlmostUsed || got[1] != Reached {
		t.Fatalf("alerts %v", got)
	}
	// A replica that starts after the budget ran out doesn't alert again.
	other := NewTracker("pod-b")
	other.Observe(key, l, &types.Spend{Pods: map[string]*types.PodSpend{"pod-a": {PeriodStart: l.Start(time.Now()).UTC().Format(time.RFC3339), USD: "1"}}})
	other.Reserve([]Item{{key, l}}, cents(1))
	if len(other.Alerts()) != 0 {
		t.Fatal("duplicate alert")
	}
}

func TestExceededMessage(t *testing.T) {
	tr := NewTracker("pod-a")
	_, err := tr.Reserve([]Item{{TenantScope("research"), limit(t, "0.50", "daily", "")}}, cents(60))
	if err == nil || err.Error()[:46] != "Daily budget of $0.50 for tenant research reac" {
		t.Fatalf("message: %v", err)
	}
}
