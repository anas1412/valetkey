// Package budget enforces dollar budgets on keys and tenants. Before a request it
// reserves the request's worst-case cost; afterwards it charges the real cost, so
// spend never passes a budget. Spend is kept in CRD status, not a database.
package budget

import (
	"fmt"
	"sync"
	"time"

	"github.com/anas1412/valetkey/internal/pricing"
	"github.com/anas1412/valetkey/internal/types"
)

const (
	Daily   = "daily"
	Weekly  = "weekly"
	Monthly = "monthly"
)

// Limit is a parsed Budget.
type Limit struct {
	Amount pricing.USD
	Period string
	Loc    *time.Location
}

// Parse reads a CRD budget; nil means no budget.
func Parse(b *types.Budget) (*Limit, error) {
	if b == nil {
		return nil, nil
	}
	amount, err := pricing.ParseUSD(b.USD)
	if err != nil {
		return nil, err
	}
	l := &Limit{Amount: amount, Period: b.Period, Loc: time.UTC}
	switch l.Period {
	case "":
		l.Period = Monthly
	case Daily, Weekly, Monthly:
	default:
		return nil, fmt.Errorf("budget period must be daily, weekly or monthly, not %q", b.Period)
	}
	if b.Timezone != "" {
		if l.Loc, err = time.LoadLocation(b.Timezone); err != nil {
			return nil, fmt.Errorf("budget timezone: %w", err)
		}
	}
	return l, nil
}

// untracked is the period used to report spend for scopes without a budget.
var untracked = Limit{Period: Monthly, Loc: time.UTC}

// Start is the beginning of the period containing t: midnight, Monday or the 1st,
// in the budget's time zone.
func (l Limit) Start(t time.Time) time.Time {
	t = t.In(l.Loc)
	y, m, d := t.Date()
	switch l.Period {
	case Daily:
		return time.Date(y, m, d, 0, 0, 0, 0, l.Loc)
	case Weekly:
		back := (int(t.Weekday()) + 6) % 7 // days since Monday
		return time.Date(y, m, d-back, 0, 0, 0, 0, l.Loc)
	default:
		return time.Date(y, m, 1, 0, 0, 0, 0, l.Loc)
	}
}

// End is when the period containing t ends and the budget resets.
func (l Limit) End(t time.Time) time.Time {
	s := l.Start(t)
	switch l.Period {
	case Daily:
		return s.AddDate(0, 0, 1)
	case Weekly:
		return s.AddDate(0, 0, 7)
	default:
		return s.AddDate(0, 1, 0)
	}
}

var parsed sync.Map // types.Budget → *Limit

// ParseCached is Parse for the request path; time zones are loaded once.
func ParseCached(b *types.Budget) *Limit {
	if b == nil {
		return nil
	}
	if l, ok := parsed.Load(*b); ok {
		return l.(*Limit)
	}
	l, err := Parse(b)
	if err != nil {
		return nil // the tenant reports the error in its status
	}
	parsed.Store(*b, l)
	return l
}
