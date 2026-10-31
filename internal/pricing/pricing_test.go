package pricing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anas1412/valetkey/internal/types"
)

// Recorded shapes of the four sources, trimmed to what the tests need.
const (
	fixtureModelsDev = `{
	  "deepseek": {"api": "https://api.deepseek.com", "models": {
	    "deepseek-v4-pro": {"cost": {"input": 0.66, "output": 1.98, "reasoning": 1.98, "cache_read": 0.022}, "limit": {"output": 393216}}}},
	  "openai": {"models": {
	    "gpt-test": {"cost": {"input": 10, "output": 50, "cache_read": 1, "cache_write": 12.5,
	      "tiers": [{"input": 20, "output": 75, "cache_read": 2, "cache_write": 25, "tier": {"type": "context", "size": 272000}}]}, "limit": {"output": 128000}}}},
	  "anthropic": {"models": {
	    "claude-sonnet-4-5": {"cost": {"input": 3, "output": 15, "cache_read": 0.3, "cache_write": 3.75}, "limit": {"output": 64000}}}}
	}`
	fixtureOpenRouter = `{"data": [
	  {"id": "anthropic/claude-sonnet-4.5", "pricing": {"prompt": "0.000003", "input_cache_write_1h": "0.000006",
	    "overrides": [{"min_prompt_tokens": 200000, "prompt": "0.000006", "completion": "0.0000225"}]}},
	  {"id": "openai/gpt-test", "pricing": {"prompt": "0.000012"}}
	]}`
	fixtureDeepSeek = "(2) Off-peak rates are half of the peak rates. Peak hours are 01:00 - 04:00 and 06:00 - 10:00 UTC, " +
		"Monday through Friday, excluding Chinese public holidays. All other hours are off-peak, including weekends and Chinese public holidays in full."
	fixtureHolidays = `{"days": [{"date": "2026-10-01", "isOffDay": true}, {"date": "2026-10-02", "isOffDay": true}, {"date": "2026-10-10", "isOffDay": false}]}`
)

func fixtureCatalog(t *testing.T, deepseekPage string) *Catalog {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/models.json":
			w.Write([]byte(fixtureModelsDev))
		case r.URL.Path == "/openrouter":
			w.Write([]byte(fixtureOpenRouter))
		case r.URL.Path == "/deepseek.md":
			w.Write([]byte(deepseekPage))
		case r.URL.Path == "/2026.json":
			w.Write([]byte(fixtureHolidays))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	cat, err := Fetcher{ModelsDev: srv.URL + "/models.json", OpenRouter: srv.URL + "/openrouter",
		DeepSeek: srv.URL + "/deepseek.md", Holidays: srv.URL + "/%d.json"}.Fetch(context.Background(), time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func usd(t *testing.T, s string) USD {
	t.Helper()
	u, err := ParseUSD(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestDeepSeekPeakHours(t *testing.T) {
	cat := fixtureCatalog(t, fixtureDeepSeek)
	p, ok := cat.Lookup(cat.ProviderFor("https://api.deepseek.com/v1"), "deepseek-v4-pro")
	if !ok {
		t.Fatal("deepseek-v4-pro not found")
	}
	at := func(s string) time.Time { tt, _ := time.Parse(time.RFC3339, s); return tt }
	for _, c := range []struct {
		when   string
		input  string // per 1M
		period string
	}{
		{"2026-11-02T00:59:00Z", "0.66", OffPeak}, // Monday
		{"2026-11-02T01:00:00Z", "1.32", Peak},
		{"2026-11-02T03:59:00Z", "1.32", Peak},
		{"2026-11-02T04:00:00Z", "0.66", OffPeak},
		{"2026-11-02T06:00:00Z", "1.32", Peak},
		{"2026-11-02T10:00:00Z", "0.66", OffPeak},
		{"2026-10-31T02:00:00Z", "0.66", OffPeak}, // Saturday
		{"2026-10-01T02:00:00Z", "0.66", OffPeak}, // Thursday, National Day
		{"2026-10-02T09:59:00Z", "0.66", OffPeak}, // Friday, holiday in Beijing
	} {
		r, period := p.At(at(c.when), 1000, cat.Calendars)
		if got := perM(r.Input); got != "$"+c.input || period != c.period {
			t.Errorf("%s: %s %s, want $%s %s", c.when, got, period, c.input, c.period)
		}
	}
	if next := p.NextChange(at("2026-11-02T00:30:00Z")); !next.Equal(at("2026-11-02T01:00:00Z")) {
		t.Errorf("next change %v", next)
	}
}

func TestDeepSeekPageUnreadableUsesBuiltInSchedule(t *testing.T) {
	cat := fixtureCatalog(t, "<html>nothing here</html>")
	p, _ := cat.Lookup("deepseek", "deepseek-v4-pro")
	if r, period := p.At(time.Date(2026, 11, 2, 2, 0, 0, 0, time.UTC), 10, cat.Calendars); perM(r.Input) != "$1.32" || period != Peak {
		t.Fatalf("fallback schedule: %s %s", perM(r.Input), period)
	}
	for _, s := range cat.Sources {
		if s.Name == "DeepSeek pricing page" && (s.OK || s.Error == "") {
			t.Fatalf("source should report the failure: %+v", s)
		}
	}
}

func TestCostWithCacheTiersAndReasoning(t *testing.T) {
	cat := fixtureCatalog(t, fixtureDeepSeek)
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

	gpt, _ := cat.Lookup("openai", "gpt-test-2026-01-01") // date suffix falls back
	r, _ := gpt.At(now, 1000, cat.Calendars)
	// 200 uncached × $10/M + 800 cached × $1/M + 300 output × $50/M
	if got := r.Cost(Tokens{Input: 200, CacheRead: 800, Output: 300}); got != usd(t, "0.0178") {
		t.Errorf("openai cost %s", got)
	}
	long, _ := gpt.At(now, 300_000, cat.Calendars)
	if perM(long.Input) != "$20.00" {
		t.Errorf("context tier input %s", perM(long.Input))
	}
	if gpt.Warning == "" {
		t.Error("expected a models.dev / OpenRouter disagreement warning ($10 vs $12)")
	}

	claude, _ := cat.Lookup("anthropic", "claude-sonnet-4-5")
	cr, _ := claude.At(now, 1000, cat.Calendars)
	if perM(cr.CacheWrite5m) != "$3.75" || perM(cr.CacheWrite1h) != "$6.00" {
		t.Errorf("anthropic cache writes %s / %s", perM(cr.CacheWrite5m), perM(cr.CacheWrite1h))
	}
	// 10 × $3 + 100 × $3.75 + 200 × $6 + 20 × $15, per million
	if got := cr.Cost(Tokens{Input: 10, CacheWrite5m: 100, CacheWrite1h: 200, Output: 20}); got != usd(t, "0.001905") {
		t.Errorf("anthropic cost %s", got)
	}
	if len(claude.Tiers) != 1 || claude.Tiers[0].Above != 199_999 {
		t.Errorf("tier from OpenRouter: %+v", claude.Tiers)
	}

	ds, _ := cat.Lookup("deepseek", "deepseek-v4-pro")
	dr, _ := ds.At(now, 10, cat.Calendars)
	if got := dr.Cost(Tokens{Input: 1_000_000, Output: 1_000_000, Reasoning: 400_000}); got != usd(t, "2.64") {
		t.Errorf("deepseek cost %s", got)
	}
	if w := ds.Worst(now, 1000, 393216, cat.Calendars); w != usd(t, "0.00066")+393216*dr.Output {
		t.Errorf("worst case %s", w)
	}
}

func TestOverrideWins(t *testing.T) {
	s := &Store{}
	s.set(fixtureCatalog(t, fixtureDeepSeek))
	spec := types.ChannelSpec{BaseURL: "https://api.deepseek.com", Pricing: &types.ChannelPricing{
		Models: map[string]types.ModelPrice{"deepseek-v4-pro": {Input: "0.5", Output: "1"}}}}
	p, ok := s.Resolve(spec, "deepseek-v4-pro")
	if !ok || p.Source != "channel override" || perM(p.Rates.Input) != "$0.50" || len(p.Windows) != 0 {
		t.Fatalf("override: %+v", p)
	}
	if _, ok := s.Resolve(types.ChannelSpec{BaseURL: "http://vllm.local:8000"}, "llama"); ok {
		t.Fatal("unknown provider should have no price")
	}
	if p, ok := s.Resolve(types.ChannelSpec{BaseURL: "http://gw.internal", Pricing: &types.ChannelPricing{Provider: "anthropic"}}, "claude-sonnet-4.5"); !ok || perM(p.Rates.Input) != "$3.00" {
		t.Fatalf("explicit provider with dotted id: %v %+v", ok, p)
	}
	if err := ValidateOverrides(&types.ChannelPricing{Models: map[string]types.ModelPrice{"x": {Input: "abc", Output: "1"}}}); err == nil {
		t.Fatal("bad override accepted")
	}
}

func TestMoney(t *testing.T) {
	for in, want := range map[string]string{"20": "$20.00", "0.5": "$0.50", "0.000001": "$0.000001", "12.3401": "$12.3401"} {
		if got := usd(t, in).String(); got != want {
			t.Errorf("%s → %s, want %s", in, got, want)
		}
	}
	if d := usd(t, "12.000000340001").Decimal(); d != "12.000000340001" {
		t.Errorf("decimal %s", d)
	}
	if _, err := ParseUSD("-1"); err == nil {
		t.Error("negative accepted")
	}
	if !strings.HasPrefix(perM(PerMillion(0.022)), "$0.022") {
		t.Errorf("per million %s", perM(PerMillion(0.022)))
	}
}

func TestSnapshotDecodes(t *testing.T) {
	cat, err := Decode(snapshot)
	if err != nil || len(cat.Providers) < 50 {
		t.Fatalf("snapshot: %v, %d providers", err, len(cat.Providers))
	}
}
