// Package relay forwards tenant requests to upstream providers.
package relay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/anas1412/valetkey/internal/auth"
	"github.com/anas1412/valetkey/internal/budget"
	"github.com/anas1412/valetkey/internal/metrics"
	"github.com/anas1412/valetkey/internal/pricing"
	"github.com/anas1412/valetkey/internal/registry"
	"github.com/anas1412/valetkey/internal/stats"
)

const (
	maxBody     = 20 << 20
	maxAttempts = 3
)

// Upstream headers returned to the client.
var forwardResponseHeaders = []string{"Content-Type", "Cache-Control", "Retry-After", "X-Request-Id", "Request-Id"}

type Relay struct {
	Registry   *registry.Registry
	Limiter    *RateLimiter
	Metrics    *metrics.Metrics
	Stats      *stats.Stats
	Transports *Transports
	Prices     *pricing.Store  // optional: no costs without it
	Budgets    *budget.Tracker // optional: no budgets without it
	// OnKeyRejected is told when a provider answers 401: the provider's key is bad.
	OnKeyRejected func(provider, reason string)
}

// result is the outcome of one relayed request, for metrics and the activity log.
type result struct {
	status        int
	provider      string
	model         string
	input, output int
	latency, ttfb time.Duration
	errorType     string
	retries       int
	cost          pricing.USD
	costNote      string
}

func (rl *Relay) ChatCompletions(w http.ResponseWriter, r *http.Request) { rl.handle(w, r, OpenAI) }
func (rl *Relay) Messages(w http.ResponseWriter, r *http.Request)        { rl.handle(w, r, Anthropic) }
func (rl *Relay) Embeddings(w http.ResponseWriter, r *http.Request)      { rl.handle(w, r, Embeddings) }
func (rl *Relay) Responses(w http.ResponseWriter, r *http.Request)       { rl.handle(w, r, Responses) }

func (rl *Relay) handle(w http.ResponseWriter, r *http.Request, api API) {
	start := time.Now()
	route := auth.RouteFrom(r.Context())
	fail := func(status int, kind, msg, model string) {
		writeError(w, api, status, kind, msg)
		rl.record(route, result{status: status, provider: "none", model: model, latency: time.Since(start), errorType: kind})
	}

	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			fail(http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds 20 MiB", "unknown")
			return
		}
		fail(http.StatusBadRequest, "invalid_request_error", "could not read request body", "unknown")
		return
	}
	var body map[string]any
	if json.Unmarshal(raw, &body) != nil || body == nil {
		fail(http.StatusBadRequest, "invalid_request_error", "request body must be valid JSON", "unknown")
		return
	}
	model, _ := body["model"].(string)
	if model == "" {
		fail(http.StatusBadRequest, "invalid_request_error", `"model" is required`, "unknown")
		return
	}
	if route.Models != nil && !slices.Contains(route.Models, model) {
		fail(http.StatusForbidden, "model_not_allowed", fmt.Sprintf("this key may not use model %q", model), model)
		return
	}

	limitKey := route.Namespace + "/" + route.ProxyKeyName
	if route.RPM > 0 || route.TPM > 0 {
		var rpmRatio, tpmRatio float64
		if g, ok := rl.Registry.Tenant(route.Tenant); ok {
			rpmRatio, tpmRatio = g.Spec.RPMRatio, g.Spec.TPMRatio
		}
		if wait, tier := rl.Limiter.Acquire(limitKey, route.RPM, route.TPM, rpmRatio, tpmRatio); wait > 0 {
			rl.Metrics.RateLimited.WithLabelValues(route.Tenant, tier).Inc()
			w.Header().Set("Retry-After", strconv.Itoa(wait))
			fail(http.StatusTooManyRequests, "rate_limit_exceeded", strings.ToUpper(tier)+" limit reached for this key", model)
			return
		}
	}

	candidates := Candidates(rl.Registry, route.Tenant, model, api)
	if len(candidates) > maxAttempts {
		candidates = candidates[:maxAttempts]
	}
	if len(candidates) == 0 {
		fail(http.StatusServiceUnavailable, "no_provider_available",
			fmt.Sprintf("no enabled provider in tenant %q serves model %q", route.Tenant, model), model)
		return
	}

	cost, candidates, cerr := rl.prepareCost(route, api, body, len(raw), model, candidates, start)
	if cerr != nil {
		fail(cerr.status, cerr.kind, cerr.message, model)
		return
	}
	settled := false
	defer func() {
		if !settled {
			cost.release()
		}
	}()

	lastStatus, lastType := http.StatusBadGateway, "upstream_error"
	for attempt, entry := range candidates {
		last := attempt == len(candidates)-1
		a, err := adapt(entry, api, body, r.Header, route.MaxTokens)
		if err != nil {
			var re *relayError
			if errors.As(err, &re) {
				fail(re.status, re.kind, re.message, model)
				return
			}
			fail(http.StatusInternalServerError, "internal_error", err.Error(), model)
			return
		}

		req, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, a.url, bytes.NewReader(a.body))
		req.Header = a.headers
		upstreamStart := time.Now()
		resp, err := rl.Transports.Client(entry.Provider).Do(req)
		if err != nil {
			if r.Context().Err() != nil {
				return // client went away
			}
			rl.Metrics.ProviderErrors.WithLabelValues(entry.Name()).Inc()
			lastStatus, lastType = http.StatusBadGateway, "upstream_error"
			slog.Warn("upstream failed", "provider", entry.Name(), "key", limitKey, "err", err)
			continue
		}

		// 401 means the provider's own key is bad, not the client's: stop sending it traffic
		// and use the next provider. 403 can be about one model, so it only moves on.
		if resp.StatusCode == http.StatusUnauthorized || (!last && resp.StatusCode == http.StatusForbidden) {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			rl.Metrics.ProviderErrors.WithLabelValues(entry.Name()).Inc()
			lastStatus, lastType = http.StatusBadGateway, "upstream_auth_error"
			if resp.StatusCode == http.StatusUnauthorized && rl.OnKeyRejected != nil {
				rl.OnKeyRejected(entry.Name(), "provider rejected the API key (HTTP 401)")
			}
			continue
		}

		// Retry rate limits and server errors on the next provider, before anything reaches the client.
		if !last && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			rl.Metrics.ProviderErrors.WithLabelValues(entry.Name()).Inc()
			lastStatus, lastType = resp.StatusCode, "upstream_error"
			if resp.StatusCode == http.StatusTooManyRequests {
				lastType = "upstream_rate_limited"
			}
			continue
		}

		ttfb := time.Since(start)
		usage := forward(w, resp, a)
		resp.Body.Close()
		rl.Metrics.UpstreamDuration.WithLabelValues(entry.Name(), model).Observe(time.Since(upstreamStart).Seconds())
		if route.TPM > 0 {
			rl.Limiter.RecordTokens(limitKey, usage.InputTotal()+usage.Output)
		}
		res := result{status: resp.StatusCode, provider: entry.Name(), model: model, input: usage.InputTotal(), output: usage.Output,
			latency: time.Since(start), ttfb: ttfb, retries: attempt}
		res.cost, res.costNote = cost.settle(entry, model, resp.StatusCode, usage)
		settled = true
		if resp.StatusCode >= 400 {
			rl.Metrics.ProviderErrors.WithLabelValues(entry.Name()).Inc()
			res.errorType = "upstream_status"
		}
		rl.record(route, res)
		return
	}
	fail(lastStatus, lastType, "all upstream providers failed", model)
}

// forward streams (or buffers and translates) the upstream response to the client.
func forward(w http.ResponseWriter, resp *http.Response, a *adaptation) *Usage {
	usage := &Usage{}
	for _, h := range forwardResponseHeaders {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}

	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		var tr StreamTranslator
		if a.newStream != nil {
			tr = a.newStream()
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(resp.StatusCode)
		flusher, _ := w.(http.Flusher)
		buf := make([]byte, 32<<10)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				usage.FeedSSE(chunk)
				if tr != nil {
					chunk = tr.Push(chunk)
				}
				if _, werr := io.WriteString(w, chunk); werr != nil {
					return usage
				}
				if flusher != nil {
					flusher.Flush()
				}
			}
			if err != nil {
				break
			}
		}
		usage.EndSSE()
		if tr != nil {
			_, _ = io.WriteString(w, tr.End())
		}
		return usage
	}

	raw, _ := io.ReadAll(resp.Body)
	usage.FeedJSON(raw)
	if a.translateJSON != nil {
		// Non-JSON bodies (e.g. a gateway's HTML error page) pass through untouched.
		if out, err := a.translateJSON(raw, resp.StatusCode); err == nil {
			raw = out
			w.Header().Set("Content-Type", "application/json")
		}
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(raw)
	return usage
}

func (rl *Relay) record(route registry.Route, r result) {
	m := rl.Metrics
	m.Requests.WithLabelValues(r.provider, r.model, route.Tenant, route.Namespace, route.ProxyKeyName, strconv.Itoa(r.status)).Inc()
	if r.input > 0 {
		m.Tokens.WithLabelValues(r.provider, r.model, route.Tenant, "input").Add(float64(r.input))
	}
	if r.output > 0 {
		m.Tokens.WithLabelValues(r.provider, r.model, route.Tenant, "output").Add(float64(r.output))
	}
	m.RequestDuration.WithLabelValues(r.provider, r.model).Observe(r.latency.Seconds())
	if r.ttfb > 0 {
		m.TTFB.WithLabelValues(r.provider, r.model).Observe(r.ttfb.Seconds())
	}
	if r.errorType != "" {
		m.Errors.WithLabelValues(r.provider, r.model, route.Tenant, r.errorType).Inc()
	}
	if r.retries > 0 {
		m.Retries.WithLabelValues(r.provider).Add(float64(r.retries))
	}
	if r.cost > 0 {
		m.Cost.WithLabelValues(r.provider, r.model, route.Tenant, route.Namespace, route.ProxyKeyName).Add(r.cost.Float())
	}
	rl.Stats.RecordRequest(stats.RequestEntry{
		Time: time.Now().UnixMilli(), Namespace: route.Namespace, Key: route.ProxyKeyName, Tenant: route.Tenant,
		Provider: r.provider, Model: r.model, Status: r.status, InputTokens: r.input, OutputTokens: r.output,
		LatencyMs: r.latency.Milliseconds(), Error: r.errorType, CostUSD: r.cost.Float(), CostNote: r.costNote,
	})
}

// Models lists the models the calling key can use: its tenant's provider models,
// narrowed by the key's allowlist.
func (rl *Relay) Models(w http.ResponseWriter, r *http.Request) {
	route := auth.RouteFrom(r.Context())
	seen := map[string]string{}
	var order []string
	for _, e := range rl.Registry.TenantProviders(route.Tenant) {
		ids := slices.Clone(e.Provider.Spec.Models)
		for m := range e.Provider.Spec.ModelMapping {
			ids = append(ids, m)
		}
		for _, id := range ids {
			if _, ok := seen[id]; !ok && (route.Models == nil || slices.Contains(route.Models, id)) {
				seen[id] = e.Provider.Spec.Type
				order = append(order, id)
			}
		}
	}
	data := make([]map[string]any, 0, len(order))
	for _, id := range order {
		data = append(data, map[string]any{"id": id, "object": "model", "created": 0, "owned_by": seen[id]})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": data})
}
