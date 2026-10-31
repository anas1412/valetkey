package relay

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/anas1412/valetkey/internal/budget"
	"github.com/anas1412/valetkey/internal/pricing"
	"github.com/anas1412/valetkey/internal/registry"
)

// Output bound for worst-case reservations when neither the request (max_tokens)
// nor the price catalogue says how long the answer can be.
const defaultMaxOutput = 131_072

// anthropicDefaultMaxTokens is the cap sent when an OpenAI client without max_tokens
// is translated for an Anthropic channel (Anthropic requires one).
const anthropicDefaultMaxTokens = 4096

// costing prices one request and holds its budget reservation.
type costing struct {
	rl                      *Relay
	start                   time.Time
	route                   registry.Route
	res                     *budget.Reservation
	budgeted                bool
	api                     API
	inputBound, outputLimit int // outputLimit 0 = take the model's max output
}

// upstreamModel is the model name the channel is billed for.
func upstreamModel(e registry.ChannelEntry, model string) string {
	if m := e.Channel.Spec.ModelMapping[model]; m != "" {
		return m
	}
	return model
}

// prepareCost reserves the request's worst-case cost on the key's and tenant's
// budgets. With a budget, channels without a known price are dropped. It returns
// the channels left to try, or a client error.
func (rl *Relay) prepareCost(route registry.Route, api API, body map[string]any, bodySize int, model string,
	candidates []registry.ChannelEntry, start time.Time) (*costing, []registry.ChannelEntry, *relayError) {
	if rl.Prices == nil || rl.Budgets == nil {
		return nil, candidates, nil
	}
	items := []budget.Item{{Scope: budget.KeyScope(route.Namespace, route.ProxyKeyName), Limit: route.Budget}}
	var tenantLimit *budget.Limit
	if g, ok := rl.Registry.Group(route.Group); ok {
		tenantLimit = budget.ParseCached(g.Spec.Budget)
	}
	items = append(items, budget.Item{Scope: budget.TenantScope(route.Group), Limit: tenantLimit})
	modelLimit := route.ModelBudgets[model]
	if modelLimit != nil {
		items = append(items, budget.Item{Scope: budget.KeyModelScope(route.Namespace, route.ProxyKeyName, model), Limit: modelLimit})
	}
	budgeted := route.Budget != nil || tenantLimit != nil || modelLimit != nil
	if budgeted && api == OpenAI && body["stream"] == true {
		// OpenAI-style streams only report usage when asked; without it the request
		// would be charged its worst case.
		opts, _ := body["stream_options"].(map[string]any)
		if opts == nil {
			opts = map[string]any{}
		}
		opts["include_usage"] = true
		body["stream_options"] = opts
	}

	// Every token is at least one byte, so the body size bounds the input; the
	// margin covers the few tokens a chat template adds.
	c := &costing{rl: rl, start: start, route: route, budgeted: budgeted, api: api, inputBound: bodySize + 256, outputLimit: requestedMaxTokens(body)}
	if c.outputLimit == 0 {
		c.outputLimit = route.MaxTokens
	}
	cal := rl.Prices.Catalog().Calendars
	var worst pricing.USD
	priced := candidates[:0:0]
	for _, e := range candidates {
		p, ok := rl.Prices.Resolve(e.Channel.Spec, upstreamModel(e, model))
		if !ok {
			if !budgeted {
				priced = append(priced, e)
			}
			continue
		}
		priced = append(priced, e)
		worst = max(worst, p.Worst(start, c.inputBound, c.outputBound(e, p), cal))
	}
	if len(priced) == 0 {
		return nil, nil, &relayError{status: http.StatusForbidden, kind: "model_not_priced",
			message: fmt.Sprintf("this key has a budget, but no price is known for model %q on its providers; set spec.pricing on the channel", model)}
	}
	res, err := rl.Budgets.Reserve(items, worst)
	if err != nil {
		var ex *budget.ExceededError
		if errors.As(err, &ex) {
			rl.Metrics.BudgetRejected.WithLabelValues(route.Group, ex.Scope.Kind).Inc()
		}
		return nil, nil, &relayError{status: http.StatusTooManyRequests, kind: "insufficient_quota", message: err.Error()}
	}
	c.res = res
	return c, priced, nil
}

// outputBound is the most output tokens the request can produce on channel e.
func (c *costing) outputBound(e registry.ChannelEntry, p pricing.Price) int {
	switch {
	case c.api == Embeddings:
		return 0
	case c.outputLimit > 0:
		return c.outputLimit
	case c.api == OpenAI && e.Channel.Spec.Type == "anthropic":
		return anthropicDefaultMaxTokens // the translation sends this cap
	case p.MaxOutput > 0:
		return p.MaxOutput
	default:
		return defaultMaxOutput
	}
}

// settle charges the request's real cost to the budgets and returns it, with
// "estimated", "unpriced" or "no usage" when it isn't exact.
func (c *costing) settle(e registry.ChannelEntry, model string, status int, u *Usage) (pricing.USD, string) {
	if c == nil {
		return 0, ""
	}
	p, ok := c.rl.Prices.Resolve(e.Channel.Spec, upstreamModel(e, model))
	cal := c.rl.Prices.Catalog().Calendars
	var cost pricing.USD
	note := ""
	switch {
	case !ok:
		note = "unpriced"
	case status >= 400:
		// Providers don't bill failed requests.
	case u.InputTotal() == 0 && u.Output == 0 && c.budgeted:
		// No usage reported (stream cut off): charge the worst case, never less.
		cost, note = p.Worst(c.start, c.inputBound, c.outputBound(e, p), cal), "estimated"
	case u.InputTotal() == 0 && u.Output == 0:
		note = "no usage"
	default:
		r, _ := p.At(c.start, u.InputTotal(), cal)
		cost = r.Cost(pricing.Tokens{Input: u.Input, CacheRead: u.CacheRead, CacheWrite5m: u.CacheWrite5m,
			CacheWrite1h: u.CacheWrite1h, Output: u.Output, Reasoning: u.Reasoning, AudioInput: u.AudioInput, AudioOutput: u.AudioOutput})
	}
	c.res.Commit(cost, note == "unpriced")
	c.report()
	return cost, note
}

// release drops the reservation of a request no provider answered.
func (c *costing) release() {
	if c != nil {
		c.res.Release()
	}
}

// report updates the budget gauges for the key and its tenant.
func (c *costing) report() {
	m, b := c.rl.Metrics, c.rl.Budgets
	key := budget.KeyScope(c.route.Namespace, c.route.ProxyKeyName)
	keyName := c.route.Namespace + "/" + c.route.ProxyKeyName
	m.BudgetSpent.WithLabelValues("key", keyName).Set(b.Usage(key, c.route.Budget).Spent.Float())
	if c.route.Budget != nil {
		m.BudgetLimit.WithLabelValues("key", keyName).Set(c.route.Budget.Amount.Float())
	}
	var tl *budget.Limit
	if g, ok := c.rl.Registry.Group(c.route.Group); ok {
		tl = budget.ParseCached(g.Spec.Budget)
	}
	m.BudgetSpent.WithLabelValues("tenant", c.route.Group).Set(b.Usage(budget.TenantScope(c.route.Group), tl).Spent.Float())
	if tl != nil {
		m.BudgetLimit.WithLabelValues("tenant", c.route.Group).Set(tl.Amount.Float())
	}
}

// requestedMaxTokens reads the client's output cap, if any.
func requestedMaxTokens(body map[string]any) int {
	for _, k := range []string{"max_completion_tokens", "max_tokens", "max_output_tokens"} {
		if v, ok := body[k].(float64); ok && v > 0 {
			return int(v)
		}
	}
	return 0
}
