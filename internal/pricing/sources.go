package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Default source URLs. Every one is public and needs no key.
const (
	ModelsDevURL  = "https://models.dev/api.json"
	OpenRouterURL = "https://openrouter.ai/api/v1/models"
	DeepSeekURL   = "https://api-docs.deepseek.com/quick_start/pricing.md"
	HolidaysCNURL = "https://raw.githubusercontent.com/NateScarlet/holiday-cn/master/%d.json"
)

// Fetcher downloads and merges the price sources.
type Fetcher struct {
	HTTP                                      *http.Client
	ModelsDev, OpenRouter, DeepSeek, Holidays string // URLs; empty = defaults
}

// Fetch builds a fresh catalogue. Only models.dev is required; the other sources
// add detail and are reported in Catalog.Sources when they fail.
func (f Fetcher) Fetch(ctx context.Context, now time.Time) (*Catalog, error) {
	if f.HTTP == nil {
		f.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	md := or(f.ModelsDev, ModelsDevURL)
	cat := &Catalog{Providers: map[string]map[string]Price{}, Hosts: map[string]string{}, Calendars: Calendars{}, FetchedAt: now.UTC()}

	var raw modelsDev
	if err := f.getJSON(ctx, md, &raw); err != nil {
		return nil, fmt.Errorf("models.dev: %w", err)
	}
	n := fromModelsDev(cat, raw)
	cat.Sources = append(cat.Sources, Source{Name: "models.dev", URL: md, OK: true, Note: fmt.Sprintf("%d models", n)})
	cat.Sources = append(cat.Sources, f.addOpenRouter(ctx, cat))
	cat.Sources = append(cat.Sources, f.addDeepSeek(ctx, cat))
	cat.Sources = append(cat.Sources, f.addHolidays(ctx, cat, now))
	return cat, nil
}

func (f Fetcher) getJSON(ctx context.Context, u string, v any) error {
	body, err := f.get(ctx, u)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

func (f Fetcher) get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "valetkey")
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

func or(s, def string) string {
	if s != "" {
		return s
	}
	return def
}

// --- models.dev: base prices for every provider ---

type modelsDev map[string]struct {
	API    string `json:"api"`
	Models map[string]struct {
		Cost *mdCost `json:"cost"`
		// limit.output is the most a model can answer, used for worst-case budgets.
		Limit struct {
			Output int `json:"output"`
		} `json:"limit"`
	} `json:"models"`
}

type mdRates struct {
	Input      float64  `json:"input"`
	Output     float64  `json:"output"`
	CacheRead  *float64 `json:"cache_read"`
	CacheWrite *float64 `json:"cache_write"`
	Reasoning  *float64 `json:"reasoning"`
}

type mdCost struct {
	mdRates
	Tiers []struct {
		mdRates
		Tier struct {
			Type string `json:"type"`
			Size int    `json:"size"`
		} `json:"tier"`
	} `json:"tiers"`
	Over200k *mdRates `json:"context_over_200k"`
}

// Hosts of providers whose models.dev entry has no "api" URL (they use their own SDKs).
var knownHosts = map[string]string{
	"api.openai.com":                    "openai",
	"api.anthropic.com":                 "anthropic",
	"generativelanguage.googleapis.com": "google",
	"api.groq.com":                      "groq",
	"api.mistral.ai":                    "mistral",
	"api.x.ai":                          "xai",
	"api.together.xyz":                  "togetherai",
	"api.cerebras.ai":                   "cerebras",
	"api.perplexity.ai":                 "perplexity",
	"api.cohere.com":                    "cohere",
	"api.deepinfra.com":                 "deepinfra",
}

func fromModelsDev(cat *Catalog, raw modelsDev) int {
	n := 0
	for pid, prov := range raw {
		if u, err := url.Parse(prov.API); err == nil && u.Hostname() != "" {
			cat.Hosts[strings.ToLower(u.Hostname())] = pid
		}
		for mid, m := range prov.Models {
			if m.Cost == nil {
				continue
			}
			p := Price{Rates: m.Cost.rates(pid), MaxOutput: m.Limit.Output, Source: "models.dev"}
			for _, t := range m.Cost.Tiers {
				if t.Tier.Type == "context" && t.Tier.Size > 0 {
					p.Tiers = append(p.Tiers, Tier{Above: t.Tier.Size, Rates: t.rates(pid)})
				}
			}
			if len(p.Tiers) == 0 && m.Cost.Over200k != nil {
				p.Tiers = []Tier{{Above: 200_000, Rates: m.Cost.Over200k.rates(pid)}}
			}
			if cat.Providers[pid] == nil {
				cat.Providers[pid] = map[string]Price{}
			}
			cat.Providers[pid][strings.ToLower(mid)] = p
			n++
		}
	}
	for host, pid := range knownHosts {
		if cat.Providers[pid] != nil {
			cat.Hosts[host] = pid
		}
	}
	return n
}

// rates fills prices a provider doesn't list with the input price, which never
// undercharges. Anthropic's 1 hour cache writes cost twice the input price.
func (r mdRates) rates(provider string) Rates {
	in := PerMillion(r.Input)
	out := Rates{Input: in, Output: PerMillion(r.Output), CacheRead: in, CacheWrite5m: in, CacheWrite1h: in}
	if r.CacheRead != nil {
		out.CacheRead = PerMillion(*r.CacheRead)
	}
	if r.CacheWrite != nil {
		out.CacheWrite5m, out.CacheWrite1h = PerMillion(*r.CacheWrite), PerMillion(*r.CacheWrite)
	}
	if provider == "anthropic" {
		out.CacheWrite1h = 2 * in
	}
	if r.Reasoning != nil {
		out.Reasoning = PerMillion(*r.Reasoning)
	}
	return out
}

// --- OpenRouter: 1 hour cache writes, long-context tiers and a cross-check ---

// OpenRouter resells these providers at their list price, so its numbers can be
// compared with models.dev. For open-weight models it lists other hosts' prices.
var openRouterFirstParty = map[string]string{"openai": "openai", "anthropic": "anthropic", "google": "google", "x-ai": "xai"}

type orPricing struct {
	Prompt       string `json:"prompt"`
	CacheWrite1h string `json:"input_cache_write_1h"`
	Overrides    []struct {
		MinPromptTokens int    `json:"min_prompt_tokens"`
		Prompt          string `json:"prompt"`
		Completion      string `json:"completion"`
		CacheRead       string `json:"input_cache_read"`
		CacheWrite      string `json:"input_cache_write"`
		CacheWrite1h    string `json:"input_cache_write_1h"`
	} `json:"overrides"`
}

func (f Fetcher) addOpenRouter(ctx context.Context, cat *Catalog) Source {
	src := Source{Name: "OpenRouter", URL: or(f.OpenRouter, OpenRouterURL)}
	var raw struct {
		Data []struct {
			ID      string    `json:"id"`
			Pricing orPricing `json:"pricing"`
		} `json:"data"`
	}
	if err := f.getJSON(ctx, src.URL, &raw); err != nil {
		src.Error = err.Error()
		return src
	}
	matched, disagree := 0, 0
	for _, m := range raw.Data {
		vendor, model, ok := strings.Cut(m.ID, "/")
		pid := openRouterFirstParty[vendor]
		if !ok || pid == "" || strings.Contains(model, ":") {
			continue
		}
		key := normalizeID(model)
		p, ok := cat.Providers[pid][key]
		if !ok {
			continue
		}
		matched++
		if v, err := ParseUSD(m.Pricing.CacheWrite1h); err == nil && v > 0 {
			p.Rates.CacheWrite1h = v
		}
		if len(p.Tiers) == 0 {
			for _, o := range m.Pricing.Overrides {
				if o.MinPromptTokens > 0 {
					p.Tiers = append(p.Tiers, Tier{Above: o.MinPromptTokens - 1, Rates: orRates(p.Rates, o.Prompt, o.Completion, o.CacheRead, o.CacheWrite, o.CacheWrite1h)})
				}
			}
		}
		if v, err := ParseUSD(m.Pricing.Prompt); err == nil && p.Rates.Input > 0 && differs(v, p.Rates.Input) {
			p.Warning = fmt.Sprintf("models.dev lists %s and OpenRouter %s per 1M input tokens; using models.dev", perM(p.Rates.Input), perM(v))
			disagree++
		}
		cat.Providers[pid][key] = p
	}
	src.OK = true
	src.Note = fmt.Sprintf("%d models cross-checked, %d disagree", matched, disagree)
	return src
}

func orRates(base Rates, prompt, completion, read, write, write1h string) Rates {
	r := base
	set := func(dst *USD, s string) {
		if v, err := ParseUSD(s); err == nil && s != "" {
			*dst = v
		}
	}
	set(&r.Input, prompt)
	set(&r.Output, completion)
	set(&r.CacheRead, read)
	set(&r.CacheWrite5m, write)
	set(&r.CacheWrite1h, write1h)
	return r
}

// differs reports a difference above 1%.
func differs(a, b USD) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d*100 > max(a, b)
}

func perM(u USD) string { return (u * 1_000_000).String() }

// --- DeepSeek: peak hours, read from DeepSeek's own pricing page ---

var (
	dsPeakSentence = regexp.MustCompile(`(?i)Peak hours are ([^.]*?)UTC([^.]*)\.`)
	dsRange        = regexp.MustCompile(`(\d{1,2}):(\d{2})\s*-\s*(\d{1,2}):(\d{2})`)
	dsHalf         = regexp.MustCompile(`(?i)off-peak rates are half of the peak rates`)
)

// deepSeekFallback is DeepSeek's published schedule (October 2026), used only when
// the pricing page can't be read or parsed.
var deepSeekFallback = schedule{
	days:     []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday},
	ranges:   [][2]int{{60, 240}, {360, 600}},
	factor:   2,
	holidays: "CN",
}

type schedule struct {
	days     []time.Weekday
	ranges   [][2]int // minutes after midnight UTC
	factor   int64    // peak price = off-peak price × factor
	holidays string
}

func (f Fetcher) addDeepSeek(ctx context.Context, cat *Catalog) Source {
	src := Source{Name: "DeepSeek pricing page", URL: or(f.DeepSeek, DeepSeekURL)}
	sched := deepSeekFallback
	body, err := f.get(ctx, src.URL)
	if err == nil {
		sched, err = parseDeepSeek(string(body))
	}
	if err != nil {
		src.Error = err.Error()
		src.Note = "using the built-in schedule: peak 01:00-04:00 and 06:00-10:00 UTC on weekdays, twice the off-peak price"
		sched = deepSeekFallback
	} else {
		src.OK = true
		src.Note = describe(sched)
	}
	for id, p := range cat.Providers["deepseek"] {
		p.Windows = nil
		for _, r := range sched.ranges {
			p.Windows = append(p.Windows, Window{Days: sched.days, Start: r[0], End: r[1], Rates: scale(p.Rates, sched.factor)})
		}
		p.Holidays = sched.holidays
		p.Source = "models.dev, peak hours from DeepSeek"
		cat.Providers["deepseek"][id] = p
	}
	return src
}

func parseDeepSeek(page string) (schedule, error) {
	m := dsPeakSentence.FindStringSubmatch(page)
	if m == nil {
		return schedule{}, fmt.Errorf("peak hours sentence not found")
	}
	s := schedule{factor: 2}
	for _, r := range dsRange.FindAllStringSubmatch(m[1], -1) {
		a, _ := strconv.Atoi(r[1])
		b, _ := strconv.Atoi(r[2])
		c, _ := strconv.Atoi(r[3])
		d, _ := strconv.Atoi(r[4])
		s.ranges = append(s.ranges, [2]int{a*60 + b, c*60 + d})
	}
	if len(s.ranges) == 0 {
		return schedule{}, fmt.Errorf("no peak time ranges in %q", m[0])
	}
	if strings.Contains(strings.ToLower(m[2]), "monday through friday") {
		s.days = deepSeekFallback.days
	} else {
		s.days = []time.Weekday{0, 1, 2, 3, 4, 5, 6}
	}
	if !dsHalf.MatchString(page) {
		return schedule{}, fmt.Errorf("peak/off-peak ratio not found")
	}
	if strings.Contains(strings.ToLower(page), "chinese public holidays") {
		s.holidays = "CN"
	}
	return s, nil
}

func describe(s schedule) string {
	var parts []string
	for _, r := range s.ranges {
		parts = append(parts, fmt.Sprintf("%02d:%02d-%02d:%02d", r[0]/60, r[0]%60, r[1]/60, r[1]%60))
	}
	days := "every day"
	if len(s.days) == 5 {
		days = "Monday to Friday"
	}
	out := fmt.Sprintf("peak %s UTC, %s, %d× off-peak", strings.Join(parts, " and "), days, s.factor)
	if s.holidays != "" {
		out += ", Chinese public holidays off-peak"
	}
	return out
}

func scale(r Rates, f int64) Rates {
	m := USD(f)
	return Rates{Input: r.Input * m, Output: r.Output * m, CacheRead: r.CacheRead * m,
		CacheWrite5m: r.CacheWrite5m * m, CacheWrite1h: r.CacheWrite1h * m, Reasoning: r.Reasoning * m}
}

// --- Chinese public holidays (State Council schedule, full ranges) ---

func (f Fetcher) addHolidays(ctx context.Context, cat *Catalog, now time.Time) Source {
	tmpl := or(f.Holidays, HolidaysCNURL)
	src := Source{Name: "Chinese public holidays", URL: fmt.Sprintf(tmpl, now.Year())}
	days := map[string]bool{}
	for i, year := range []int{now.Year(), now.Year() + 1} {
		var raw struct {
			Days []struct {
				Date     string `json:"date"`
				IsOffDay bool   `json:"isOffDay"`
			} `json:"days"`
		}
		if err := f.getJSON(ctx, fmt.Sprintf(tmpl, year), &raw); err != nil {
			if i == 0 {
				src.Error = err.Error()
				return src
			}
			continue // next year's schedule is published around November
		}
		for _, d := range raw.Days {
			if d.IsOffDay {
				days[d.Date] = true
			}
		}
	}
	cat.Calendars["CN"] = days
	src.OK = true
	src.Note = fmt.Sprintf("%d holidays", len(days))
	return src
}
