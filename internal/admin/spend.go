package admin

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/anas1412/valetkey/internal/budget"
	"github.com/anas1412/valetkey/internal/pricing"
	"github.com/anas1412/valetkey/internal/types"
)

// spendView is a key's or tenant's spend in the current budget period.
type spendView struct {
	Spent       float64  `json:"spent"`
	Exact       string   `json:"exact"` // exact decimal dollars
	Reserved    float64  `json:"reserved"`
	Budget      *float64 `json:"budget,omitempty"`
	Period      string   `json:"period"`
	PeriodStart string   `json:"periodStart"`
	Resets      string   `json:"resets,omitempty"`
	Requests    int64    `json:"requests"`
	Unpriced    int64    `json:"unpriced,omitempty"`
}

func (a *API) spendOf(s budget.Scope, b *types.Budget) *spendView {
	if a.Budgets == nil {
		return nil
	}
	l := budget.ParseCached(b)
	u := a.Budgets.Usage(s, l)
	v := &spendView{Spent: u.Spent.Float(), Exact: u.Spent.Decimal(), Reserved: u.Reserved.Float(), Period: budget.Monthly,
		PeriodStart: u.PeriodStart.UTC().Format(time.RFC3339), Requests: u.Requests, Unpriced: u.Unpriced}
	if l != nil {
		amount := l.Amount.Float()
		v.Budget, v.Period, v.Resets = &amount, l.Period, l.End(time.Now()).UTC().Format(time.RFC3339)
	}
	return v
}

// validBudget rejects a budget the operator would refuse anyway.
func validBudget(w http.ResponseWriter, b *types.Budget) bool {
	if _, err := budget.Parse(b); err != nil {
		writeMessage(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}

// priceView is what one channel model costs right now.
type priceView struct {
	Channel  string `json:"channel"`
	Model    string `json:"model"`
	Provider string `json:"provider,omitempty"`
	Known    bool   `json:"known"`
	Source   string `json:"source,omitempty"`
	Warning  string `json:"warning,omitempty"`
	// Dollars per million tokens, at the current time of day.
	Input        float64 `json:"input"`
	Output       float64 `json:"output"`
	CacheRead    float64 `json:"cacheRead"`
	CacheWrite5m float64 `json:"cacheWrite5m"`
	CacheWrite1h float64 `json:"cacheWrite1h"`
	// "peak" or "off-peak" for models with time-of-day prices.
	Period     string  `json:"period,omitempty"`
	NextChange string  `json:"nextChange,omitempty"`
	OffPeakIn  float64 `json:"offPeakInput,omitempty"`
}

type pricingView struct {
	FetchedAt string           `json:"fetchedAt,omitempty"`
	Refresh   string           `json:"refresh"`
	Sources   []pricing.Source `json:"sources"`
	Prices    []priceView      `json:"prices"`
}

func perM(u pricing.USD) float64 { return u.Float() * 1e6 }

func (a *API) pricing(w http.ResponseWriter, _ *http.Request) {
	if a.Prices == nil {
		writeMessage(w, http.StatusNotFound, "pricing is not enabled")
		return
	}
	cat := a.Prices.Catalog()
	now := time.Now()
	out := pricingView{Sources: cat.Sources, Prices: []priceView{}, Refresh: "off"}
	if a.Config.PricingRefresh > 0 {
		out.Refresh = strings.TrimSuffix(strings.TrimSuffix(a.Config.PricingRefresh.String(), "0s"), "0m")
	}
	if !cat.FetchedAt.IsZero() {
		out.FetchedAt = cat.FetchedAt.Format(time.RFC3339)
	}
	for _, e := range a.Registry.Channels() {
		spec := e.Channel.Spec
		provider := cat.ProviderFor(spec.BaseURL)
		if spec.Pricing != nil && spec.Pricing.Provider != "" {
			provider = spec.Pricing.Provider
		}
		for _, model := range spec.Models {
			upstream := model
			if m := spec.ModelMapping[model]; m != "" {
				upstream = m
			}
			v := priceView{Channel: e.Name(), Model: model, Provider: provider}
			if p, ok := a.Prices.Resolve(spec, upstream); ok {
				r, period := p.At(now, 1, cat.Calendars)
				v.Known, v.Source, v.Warning, v.Period = true, p.Source, p.Warning, period
				v.Input, v.Output, v.CacheRead = perM(r.Input), perM(r.Output), perM(r.CacheRead)
				v.CacheWrite5m, v.CacheWrite1h = perM(r.CacheWrite5m), perM(r.CacheWrite1h)
				if period != "" {
					v.OffPeakIn = perM(p.Rates.Input)
					if next := p.NextChange(now); !next.IsZero() {
						v.NextChange = next.Format(time.RFC3339)
					}
				}
			}
			out.Prices = append(out.Prices, v)
		}
	}
	sort.Slice(out.Prices, func(i, j int) bool {
		if out.Prices[i].Channel != out.Prices[j].Channel {
			return out.Prices[i].Channel < out.Prices[j].Channel
		}
		return out.Prices[i].Model < out.Prices[j].Model
	})
	writeJSON(w, 200, out)
}
