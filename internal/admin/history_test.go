package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anas1412/valetkey/internal/promql"
	"github.com/anas1412/valetkey/internal/reconciler"
	"github.com/anas1412/valetkey/internal/registry"
	"github.com/anas1412/valetkey/internal/stats"
)

// fakePrometheus answers instant queries with fixed totals and range queries with a
// flat series, enough to check the Overview reads (and survives restarts via) Prometheus.
func fakePrometheus(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/query":
			result := `[{"metric":{},"value":[0,"%s"]}]`
			switch {
			case strings.Contains(q, "by (provider)"):
				fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"provider":"openai"},"value":[0,"90"]},{"metric":{"provider":"anthropic"},"value":[0,"30"]}]}}`)
				return
			case strings.Contains(q, "by (proxykey_ns, proxykey_name)"):
				fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"proxykey_ns":"billing","proxykey_name":"invoice-api"},"value":[0,"42"]}]}}`)
				return
			case strings.Contains(q, "status_code"):
				result = fmt.Sprintf(result, "6")
			case strings.Contains(q, `type="input"`):
				result = fmt.Sprintf(result, "1000")
			case strings.Contains(q, `type="output"`):
				result = fmt.Sprintf(result, "250.4")
			default:
				result = fmt.Sprintf(result, "120")
			}
			fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":%s}}`, result)
		case "/api/v1/query_range":
			start, end, step := r.URL.Query().Get("start"), r.URL.Query().Get("end"), r.URL.Query().Get("step")
			var s, e, st int64
			fmt.Sscan(start, &s)
			fmt.Sscan(end, &e)
			fmt.Sscan(step, &st)
			var points []string
			for ts := s; ts <= e; ts += st {
				points = append(points, fmt.Sprintf(`[%d,"2"]`, ts))
			}
			fmt.Fprintf(w, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[%s]}]}}`, strings.Join(points, ","))
		default:
			http.NotFound(w, r)
		}
	}))
}

func overviewOf(t *testing.T, api *API, query string) map[string]any {
	t.Helper()
	mux := http.NewServeMux()
	api.Register(mux)
	req := httptest.NewRequest("GET", "http://localhost:8082/api/v1/overview"+query, nil)
	req.Header.Set("X-Valetkey-Request", "1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("overview: %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func TestOverviewFromPrometheus(t *testing.T) {
	prom := fakePrometheus(t)
	defer prom.Close()
	api := &API{Registry: registry.New(), Stats: stats.New(), Prom: promql.New(prom.URL)}
	api.Controller = nilController()

	out := overviewOf(t, api, "?range=7d")
	if out["source"] != "prometheus" || out["range"] != "7d" {
		t.Fatalf("source=%v range=%v warning=%v", out["source"], out["range"], out["warning"])
	}
	totals := out["totals"].(map[string]any)
	if totals["requests"] != 120.0 || totals["errors"] != 6.0 || totals["inputTokens"] != 1000.0 || totals["outputTokens"] != 250.0 {
		t.Fatalf("totals %v", totals)
	}
	share := out["providerShare"].(map[string]any)
	if share["openai"] != 90.0 || share["anthropic"] != 30.0 {
		t.Fatalf("share %v", share)
	}
	series := out["series"].([]any)
	if len(series) < 160 || len(series) > 170 { // 7 days of hourly buckets
		t.Fatalf("series has %d buckets", len(series))
	}
	if b := series[len(series)/2].(map[string]any); b["requests"] != 2.0 {
		t.Fatalf("bucket %v", b)
	}
	if out["stepMinutes"] != 60.0 {
		t.Fatalf("step %v", out["stepMinutes"])
	}
}

func TestOverviewFallsBackWhenPrometheusIsDown(t *testing.T) {
	api := &API{Registry: registry.New(), Stats: stats.New(), Prom: promql.New("http://127.0.0.1:1")}
	api.Controller = nilController()
	api.Stats.RecordRequest(stats.RequestEntry{Time: time.Now().UnixMilli(), Namespace: "billing", Key: "k", Provider: "openai", Status: 200, InputTokens: 3})

	out := overviewOf(t, api, "?range=30d")
	if out["source"] != "memory" || out["range"] != "24h" || out["warning"] == "" {
		t.Fatalf("source=%v range=%v warning=%v", out["source"], out["range"], out["warning"])
	}
	if out["totals"].(map[string]any)["requests"] != 1.0 {
		t.Fatalf("memory totals %v", out["totals"])
	}
}

func TestPromDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{5 * time.Minute: "5m", time.Hour: "1h", 6 * time.Hour: "6h", 30 * 24 * time.Hour: "30d"} {
		if got := promDuration(d); got != want {
			t.Errorf("promDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

// nilController has no informers yet: it reports no ProxyKeys.
func nilController() *reconciler.Controller { return &reconciler.Controller{} }
