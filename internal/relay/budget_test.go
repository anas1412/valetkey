package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/anas1412/valetkey/internal/auth"
	"github.com/anas1412/valetkey/internal/budget"
	"github.com/anas1412/valetkey/internal/metrics"
	"github.com/anas1412/valetkey/internal/pricing"
	"github.com/anas1412/valetkey/internal/registry"
	"github.com/anas1412/valetkey/internal/stats"
	"github.com/anas1412/valetkey/internal/types"
)

// A key with a $0.004 budget on a provider priced at $10/M input and $30/M output.
// Each request uses 10 input and 4 output tokens ($0.00022), but reserves its worst case.
func TestBudgetStopsBeforeOverspending(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"hi"}}],"usage":{"prompt_tokens":10,"completion_tokens":4}}`)
	}))
	defer upstream.Close()

	reg := registry.New()
	reg.UpsertProvider(&types.Provider{ObjectMeta: metav1.ObjectMeta{Name: "ok"}, Spec: types.ProviderSpec{
		Type: "openai", BaseURL: upstream.URL, Models: []string{"priced", "unpriced"}, KeySecretRef: types.SecretRef{Name: "s"},
		Pricing: &types.ProviderPricing{Models: map[string]types.ModelPrice{"priced": {Input: "10", Output: "30"}}}}}, "k")
	reg.UpsertTenant(&types.Tenant{ObjectMeta: metav1.ObjectMeta{Name: "team"}, Spec: types.TenantSpec{Providers: []string{"ok"}}})

	limit, _ := budget.Parse(&types.Budget{USD: "0.004", Period: "daily"})
	tracker, st := budget.NewTracker("test"), stats.New()
	rl := &Relay{Registry: reg, Limiter: NewRateLimiter(), Metrics: metrics.New(), Stats: st, Transports: NewTransports(),
		Prices: &pricing.Store{}, Budgets: tracker}
	route := registry.Route{Tenant: "team", Namespace: "team", ProxyKeyName: "app", Budget: limit}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rl.ChatCompletions(w, r.WithContext(auth.WithRoute(r.Context(), route)))
	}))
	defer proxy.Close()
	f := &fixture{proxy: proxy}

	accepted := 0
	var refusal map[string]any
	for range 30 {
		resp, out := f.post(t, "/v1/chat/completions", map[string]any{"model": "priced", "max_tokens": 10,
			"messages": []any{map[string]any{"role": "user", "content": "hi"}}}, nil)
		if resp.StatusCode == http.StatusTooManyRequests {
			refusal = out
			break
		}
		if resp.StatusCode != 200 {
			t.Fatalf("status %d %v", resp.StatusCode, out)
		}
		accepted++
	}
	if refusal == nil || accepted == 0 {
		t.Fatalf("never refused (accepted %d)", accepted)
	}
	if typ := refusal["error"].(map[string]any)["type"]; typ != "insufficient_quota" {
		t.Fatalf("refusal type %v", typ)
	}
	u := tracker.Usage(budget.KeyScope("team", "app"), limit)
	perRequest, _ := pricing.ParseUSD("0.00022")
	if u.Spent != perRequest*pricing.USD(accepted) || u.Spent > limit.Amount || u.Reserved != 0 {
		t.Fatalf("spent %s after %d requests (reserved %s)", u.Spent, accepted, u.Reserved)
	}
	if e := st.Requests(); len(e) < 2 || e[0].Error != "insufficient_quota" || e[1].CostUSD != 0.00022 {
		t.Fatalf("activity: %+v", e[:min(2, len(e))])
	}

	// A budget on one model of the key.
	tiny, _ := budget.Parse(&types.Budget{USD: "0.0001", Period: "daily"})
	route.Budget, route.ModelBudgets = nil, map[string]*budget.Limit{"priced": tiny}
	resp, out := f.post(t, "/v1/chat/completions", map[string]any{"model": "priced", "max_tokens": 10}, nil)
	if msg, _ := out["error"].(map[string]any)["message"].(string); resp.StatusCode != 429 || !strings.Contains(msg, "model priced on key team/app") {
		t.Fatalf("model budget: %d %v", resp.StatusCode, out)
	}
	route.ModelBudgets = nil
	route.Budget = limit

	resp, out = f.post(t, "/v1/chat/completions", map[string]any{"model": "unpriced"}, nil)
	if resp.StatusCode != http.StatusForbidden || out["error"].(map[string]any)["type"] != "model_not_priced" {
		t.Fatalf("unpriced model with a budget: %d %v", resp.StatusCode, out)
	}
}
