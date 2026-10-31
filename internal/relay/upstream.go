package relay

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/anas1412/valetkey/internal/types"
)

// JoinURL joins an API path onto a channel base URL, keeping any path prefix in the
// base (e.g. https://openrouter.ai/api). A base already ending in /v1 is not doubled.
func JoinURL(base, apiPath string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	prefix := strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(prefix, "/v1") && strings.HasPrefix(apiPath, "/v1/") {
		apiPath = apiPath[3:]
	}
	u.Path = prefix + apiPath
	return u.String(), nil
}

// Transports gives each channel an HTTP client honoring skipTLSVerify and proxyURL.
// Clients are rebuilt when the channel changes (new resourceVersion).
type Transports struct {
	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	version string
	client  *http.Client
}

var defaultClient = &http.Client{Transport: newTransport(nil, false)}

func NewTransports() *Transports { return &Transports{cache: map[string]cached{}} }

func (t *Transports) Client(ch *types.Channel) *http.Client {
	if !ch.Spec.SkipTLSVerify && ch.Spec.ProxyURL == "" {
		return defaultClient
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if c, ok := t.cache[ch.Name]; ok && c.version == ch.ResourceVersion {
		return c.client
	}
	var proxy *url.URL
	if ch.Spec.ProxyURL != "" {
		proxy, _ = url.Parse(ch.Spec.ProxyURL)
	}
	c := &http.Client{Transport: newTransport(proxy, ch.Spec.SkipTLSVerify)}
	if old, ok := t.cache[ch.Name]; ok {
		old.client.CloseIdleConnections()
	}
	t.cache[ch.Name] = cached{version: ch.ResourceVersion, client: c}
	return c
}

func newTransport(proxy *url.URL, skipTLS bool) *http.Transport {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	if proxy != nil {
		tr.Proxy = http.ProxyURL(proxy)
	}
	tr.ResponseHeaderTimeout = 10 * time.Minute
	// Ask for identity encoding so bodies can be parsed for usage and translated.
	tr.DisableCompression = true
	if skipTLS {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in per channel
	}
	return tr
}
