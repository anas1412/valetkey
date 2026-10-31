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
	tr := NewTracker()
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
	tr := NewTracker()
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
	tr := NewTracker()
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

func TestSaveAndRestore(t *testing.T) {
	tr := NewTracker()
	key, l := KeyScope("ns", "k"), limit(t, "10", "", "")
	r, _ := tr.Reserve([]Item{{key, l}}, cents(500))
	r.Commit(pricing.USD(1_234_567_891), false) // $0.001234567891

	saved := map[Scope]types.Spend{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tr.Run(ctx, time.Hour, func(_ context.Context, s Scope, sp types.Spend) error { saved[s] = sp; return nil })
	sp := saved[key]
	if sp.USD != "0.001234567891" || sp.Requests != 1 {
		t.Fatalf("saved %+v", sp)
	}

	fresh := NewTracker()
	fresh.Restore(key, l, &sp)
	fresh.Restore(key, l, &types.Spend{PeriodStart: sp.PeriodStart, USD: "9"}) // ignored: already loaded
	if u := fresh.Usage(key, l); u.Spent != pricing.USD(1_234_567_891) || u.Requests != 1 {
		t.Fatalf("restored %+v", u)
	}
	old := NewTracker()
	old.Restore(key, l, &types.Spend{PeriodStart: "2020-01-01T00:00:00Z", USD: "9"})
	if u := old.Usage(key, l); u.Spent != 0 {
		t.Fatalf("spend from an old period should not count: %+v", u)
	}
}

func TestExceededMessage(t *testing.T) {
	tr := NewTracker()
	_, err := tr.Reserve([]Item{{TenantScope("research"), limit(t, "0.50", "daily", "")}}, cents(60))
	if err == nil || err.Error()[:46] != "Daily budget of $0.50 for tenant research reac" {
		t.Fatalf("message: %v", err)
	}
}
