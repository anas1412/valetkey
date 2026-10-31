package pricing

import (
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Catalog is the merged price list from every source.
type Catalog struct {
	Providers map[string]map[string]Price `json:"providers"` // provider → model → price
	Hosts     map[string]string           `json:"hosts"`     // API host → provider
	Calendars Calendars                   `json:"calendars"`
	FetchedAt time.Time                   `json:"fetchedAt"`
	Sources   []Source                    `json:"sources"`
}

// Source is the outcome of one fetch.
type Source struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	OK    bool   `json:"ok"`
	Note  string `json:"note,omitempty"`
	Error string `json:"error,omitempty"`
}

// ProviderFor guesses the catalogue provider from a channel's base URL.
func (c *Catalog) ProviderFor(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return c.Hosts[strings.ToLower(u.Hostname())]
}

var dateSuffix = regexp.MustCompile(`-(\d{4}-\d{2}-\d{2}|\d{8}|\d{4})$`)

// Lookup finds a model's price: exact id, then with dots as dashes, then without a
// date or version suffix (gpt-4o-2024-08-06 → gpt-4o).
func (c *Catalog) Lookup(provider, model string) (Price, bool) {
	models := c.Providers[provider]
	if models == nil {
		return Price{}, false
	}
	for _, id := range []string{model, normalizeID(model), dateSuffix.ReplaceAllString(normalizeID(model), "")} {
		if p, ok := models[id]; ok {
			return p, true
		}
	}
	return Price{}, false
}

func normalizeID(id string) string { return strings.ToLower(strings.ReplaceAll(id, ".", "-")) }
