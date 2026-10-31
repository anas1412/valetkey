package admin

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/anas1412/valetkey/internal/promql"
	"github.com/anas1412/valetkey/internal/stats"
)

// historyRange is a time window the Overview can show from Prometheus.
type historyRange struct {
	span time.Duration
	step time.Duration
}

var historyRanges = map[string]historyRange{
	"24h": {24 * time.Hour, 5 * time.Minute},
	"7d":  {7 * 24 * time.Hour, time.Hour},
	"30d": {30 * 24 * time.Hour, 6 * time.Hour},
}

// usageHistory is what the Overview shows: totals, a series and the traffic share.
type usageHistory struct {
	Totals        map[string]int `json:"totals"`
	Series        []stats.Bucket `json:"series"`
	StepMinutes   int            `json:"stepMinutes"`
	ProviderShare map[string]int `json:"providerShare"`
	Cost          float64        `json:"cost"` // US dollars over the range
}

const errorStatus = `status_code=~"[45].."`

// promHistory reads usage for the range from Prometheus. Counters reset when the
// pod restarts; increase() accounts for that, so totals span restarts.
func promHistory(ctx context.Context, c *promql.Client, rangeName string) (usageHistory, error) {
	r := historyRanges[rangeName]
	now := time.Now()
	window := promDuration(r.span)
	step := promDuration(r.step)

	h := usageHistory{
		Totals:        map[string]int{},
		StepMinutes:   int(r.step.Minutes()),
		ProviderShare: map[string]int{},
	}

	totals := map[string]string{
		"requests":     fmt.Sprintf(`sum(increase(valetkey_requests_total[%s]))`, window),
		"errors":       fmt.Sprintf(`sum(increase(valetkey_requests_total{%s}[%s]))`, errorStatus, window),
		"inputTokens":  fmt.Sprintf(`sum(increase(valetkey_tokens_total{type="input"}[%s]))`, window),
		"outputTokens": fmt.Sprintf(`sum(increase(valetkey_tokens_total{type="output"}[%s]))`, window),
	}
	for name, q := range totals {
		samples, err := c.Query(ctx, q, now)
		if err != nil {
			return h, err
		}
		h.Totals[name] = sumSamples(samples)
	}

	cost, err := c.Query(ctx, fmt.Sprintf(`sum(increase(valetkey_cost_usd_total[%s]))`, window), now)
	if err != nil {
		return h, err
	}
	for _, s := range cost {
		h.Cost += s.Value
	}

	share, err := c.Query(ctx, fmt.Sprintf(`sum by (provider) (increase(valetkey_requests_total{provider!="none"}[%s]))`, window), now)
	if err != nil {
		return h, err
	}
	for _, s := range share {
		if n := int(math.Round(s.Value)); n > 0 {
			h.ProviderShare[s.Labels["provider"]] = n
		}
	}

	// One bucket per step, aligned to the step so buckets don't drift between polls.
	start := now.Add(-r.span).Truncate(r.step).Add(r.step)
	end := now.Truncate(r.step)
	buckets := map[int64]*stats.Bucket{}
	for t := start; !t.After(end); t = t.Add(r.step) {
		buckets[t.Unix()] = &stats.Bucket{Minute: t.Unix() / 60}
	}
	round := func(v float64) int { return int(math.Round(v)) }
	series := map[string]func(*stats.Bucket, float64){
		fmt.Sprintf(`sum(increase(valetkey_requests_total[%s]))`, step):                  func(b *stats.Bucket, v float64) { b.Requests = round(v) },
		fmt.Sprintf(`sum(increase(valetkey_requests_total{%s}[%s]))`, errorStatus, step): func(b *stats.Bucket, v float64) { b.Errors = round(v) },
		fmt.Sprintf(`sum(increase(valetkey_tokens_total{type="input"}[%s]))`, step):      func(b *stats.Bucket, v float64) { b.InputTokens = round(v) },
		fmt.Sprintf(`sum(increase(valetkey_tokens_total{type="output"}[%s]))`, step):     func(b *stats.Bucket, v float64) { b.OutputTokens = round(v) },
		fmt.Sprintf(`sum(increase(valetkey_cost_usd_total[%s]))`, step):                  func(b *stats.Bucket, v float64) { b.Cost = v },
	}
	for q, set := range series {
		res, err := c.QueryRange(ctx, q, start, end, r.step)
		if err != nil {
			return h, err
		}
		for _, s := range res {
			for _, p := range s.Points {
				if b := buckets[p.Time.Unix()]; b != nil {
					set(b, p.Value)
				}
			}
		}
	}
	for t := start; !t.After(end); t = t.Add(r.step) {
		h.Series = append(h.Series, *buckets[t.Unix()])
	}
	return h, nil
}

// promKeyRequests returns each key's requests over the last 30 days, by "ns/name".
func promKeyRequests(ctx context.Context, c *promql.Client) (map[string]int, error) {
	res, err := c.Query(ctx, `sum by (proxykey_ns, proxykey_name) (increase(valetkey_requests_total[30d]))`, time.Now())
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, s := range res {
		out[s.Labels["proxykey_ns"]+"/"+s.Labels["proxykey_name"]] = int(math.Round(s.Value))
	}
	return out, nil
}

// memoryHistory is the in-pod fallback: the last 24 h since the pod started.
func memoryHistory(st *stats.Stats) usageHistory {
	series := st.Series(seriesStepMinutes)
	var t stats.Bucket
	for _, b := range series {
		t.Requests += b.Requests
		t.Errors += b.Errors
		t.InputTokens += b.InputTokens
		t.OutputTokens += b.OutputTokens
		t.Cost += b.Cost
	}
	return usageHistory{
		Totals:        map[string]int{"requests": t.Requests, "errors": t.Errors, "inputTokens": t.InputTokens, "outputTokens": t.OutputTokens},
		Series:        series,
		StepMinutes:   seriesStepMinutes,
		ProviderShare: st.ProviderShare(),
		Cost:          t.Cost,
	}
}

func sumSamples(samples []promql.Sample) int {
	var total float64
	for _, s := range samples {
		total += s.Value
	}
	return int(math.Round(total))
}

// promDuration formats a duration the way PromQL range selectors expect ("5m", "6h", "30d").
func promDuration(d time.Duration) string {
	switch {
	case d%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
}
