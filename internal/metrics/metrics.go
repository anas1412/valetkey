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
	ChannelErrors    *prometheus.CounterVec
	RequestDuration  *prometheus.HistogramVec
	TTFB             *prometheus.HistogramVec
	UpstreamDuration *prometheus.HistogramVec
	ChannelsActive   *prometheus.GaugeVec
	ProxyKeysActive  *prometheus.GaugeVec
}

func New() *Metrics {
	r := prometheus.NewRegistry()
	r.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	f := factory{r}
	return &Metrics{
		Registry:         r,
		Requests:         f.counter("llmproxy_requests_total", "Total relay requests", "channel", "model", "group", "proxykey_ns", "proxykey_name", "status_code"),
		Tokens:           f.counter("llmproxy_tokens_total", "Total tokens processed", "channel", "model", "group", "type"),
		Errors:           f.counter("llmproxy_errors_total", "Total relay errors", "channel", "model", "group", "error_type"),
		RateLimited:      f.counter("llmproxy_rate_limited_total", "Total rate-limited requests", "group", "tier"),
		KeysMinted:       f.counter("llmproxy_keys_minted_total", "Total virtual keys minted", "group"),
		Retries:          f.counter("llmproxy_retries_total", "Total retry attempts", "channel"),
		ChannelErrors:    f.counter("llmproxy_channel_errors_total", "Failed upstream attempts per channel (connection errors, timeouts, 429/5xx)", "channel"),
		RequestDuration:  f.histogram("llmproxy_request_duration_seconds", "End-to-end request duration", []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120}),
		TTFB:             f.histogram("llmproxy_ttfb_seconds", "Time to first byte from upstream", []float64{0.05, 0.1, 0.5, 1, 2, 5, 10, 30}),
		UpstreamDuration: f.histogram("llmproxy_upstream_duration_seconds", "Upstream request duration", []float64{0.5, 1, 5, 10, 30, 60, 120, 300}),
		ChannelsActive:   f.gauge("llmproxy_channels_active", "Number of usable channels by type", "type"),
		ProxyKeysActive:  f.gauge("llmproxy_proxykeys_active", "Number of active proxy keys per group", "group"),
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
	h := prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: name, Help: help, Buckets: buckets}, []string{"channel", "model"})
	f.r.MustRegister(h)
	return h
}
