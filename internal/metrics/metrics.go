// Package metrics defines the Prometheus metrics served on /metrics.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	Registry *prometheus.Registry

	Requests         *prometheus.CounterVec
	Tokens           *prometheus.CounterVec
	Errors           *prometheus.CounterVec
	RateLimited      *prometheus.CounterVec
	KeysMinted       *prometheus.CounterVec
	Retries          *prometheus.CounterVec
	ProviderErrors   *prometheus.CounterVec
	RequestDuration  *prometheus.HistogramVec
	TTFB             *prometheus.HistogramVec
	UpstreamDuration *prometheus.HistogramVec
	ProvidersActive  *prometheus.GaugeVec
	ProxyKeysActive  *prometheus.GaugeVec
	Cost             *prometheus.CounterVec
	BudgetRejected   *prometheus.CounterVec
	BudgetLimit      *prometheus.GaugeVec
	BudgetSpent      *prometheus.GaugeVec
	PricesFetched    prometheus.Gauge
	PriceSourceUp    *prometheus.GaugeVec
}

func New() *Metrics {
	r := prometheus.NewRegistry()
	r.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	f := factory{r}
	return &Metrics{
		Registry:         r,
		Requests:         f.counter("valetkey_requests_total", "Total relay requests", "provider", "model", "tenant", "proxykey_ns", "proxykey_name", "status_code"),
		Tokens:           f.counter("valetkey_tokens_total", "Total tokens processed", "provider", "model", "tenant", "type"),
		Errors:           f.counter("valetkey_errors_total", "Total relay errors", "provider", "model", "tenant", "error_type"),
		RateLimited:      f.counter("valetkey_rate_limited_total", "Total rate-limited requests", "tenant", "tier"),
		KeysMinted:       f.counter("valetkey_keys_minted_total", "Total virtual keys minted", "tenant"),
		Retries:          f.counter("valetkey_retries_total", "Total retry attempts", "provider"),
		ProviderErrors:   f.counter("valetkey_provider_errors_total", "Failed upstream attempts per provider (connection errors, timeouts, 429/5xx)", "provider"),
		RequestDuration:  f.histogram("valetkey_request_duration_seconds", "End-to-end request duration", []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120}),
		TTFB:             f.histogram("valetkey_ttfb_seconds", "Time to first byte from upstream", []float64{0.05, 0.1, 0.5, 1, 2, 5, 10, 30}),
		UpstreamDuration: f.histogram("valetkey_upstream_duration_seconds", "Upstream request duration", []float64{0.5, 1, 5, 10, 30, 60, 120, 300}),
		ProvidersActive:  f.gauge("valetkey_providers_active", "Number of usable providers by type", "type"),
		ProxyKeysActive:  f.gauge("valetkey_proxykeys_active", "Number of active proxy keys per tenant", "tenant"),
		Cost:             f.counter("valetkey_cost_usd_total", "Cost of relayed requests in US dollars", "provider", "model", "tenant", "proxykey_ns", "proxykey_name"),
		BudgetRejected:   f.counter("valetkey_budget_rejections_total", "Requests refused because a budget would be exceeded", "tenant", "scope"),
		BudgetLimit:      f.gauge("valetkey_budget_usd", "Budget per period in US dollars", "scope", "name"),
		BudgetSpent:      f.gauge("valetkey_budget_spent_usd", "Spend in the current budget period in US dollars", "scope", "name"),
		PricesFetched:    f.gauge("valetkey_pricing_fetched_timestamp_seconds", "When the price list in use was fetched (Unix time)").WithLabelValues(),
		PriceSourceUp:    f.gauge("valetkey_pricing_source_up", "Whether the last fetch from a price source worked (1) or not (0)", "source"),
	}
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}

type factory struct{ r *prometheus.Registry }

func (f factory) counter(name, help string, labels ...string) *prometheus.CounterVec {
	c := prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, labels)
	f.r.MustRegister(c)
	return c
}

func (f factory) gauge(name, help string, labels ...string) *prometheus.GaugeVec {
	g := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: name, Help: help}, labels)
	f.r.MustRegister(g)
	return g
}

func (f factory) histogram(name, help string, buckets []float64) *prometheus.HistogramVec {
	h := prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: name, Help: help, Buckets: buckets}, []string{"provider", "model"})
	f.r.MustRegister(h)
	return h
}
