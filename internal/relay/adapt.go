package relay

import (
	"encoding/json"
	"net/http"

	"github.com/anas1412/valetkey/internal/registry"
)

const anthropicVersion = "2023-06-01"

// Client headers forwarded upstream. Everything else (cookies, auth, x-forwarded-*) is dropped.
var forwardRequestHeaders = []string{"Accept", "User-Agent", "Anthropic-Version", "Anthropic-Beta", "OpenAI-Beta"}

// StreamTranslator incrementally converts one SSE format into another.
type StreamTranslator interface {
	Push(chunk string) string
	End() string
}

// adaptation is a client request adapted for one channel.
type adaptation struct {
	url     string
	headers http.Header
	body    []byte
	// Non-nil when the response must be translated back to the client's shape.
	translateJSON func(body []byte, status int) ([]byte, error)
	newStream     func() StreamTranslator
}

// adapt builds the upstream request: model mapping, auth, and translation when an
// OpenAI client hits an Anthropic channel.
func adapt(e registry.ChannelEntry, api API, body map[string]any, in http.Header) (*adaptation, error) {
	spec := e.Channel.Spec
	out := make(map[string]any, len(body))
	for k, v := range body {
		out[k] = v
	}
	if model, _ := body["model"].(string); spec.ModelMapping[model] != "" {
		out["model"] = spec.ModelMapping[model]
	}

	h := http.Header{}
	for _, name := range forwardRequestHeaders {
		if v := in.Get(name); v != "" {
			h.Set(name, v)
		}
	}
	h.Set("Content-Type", "application/json")

	a := &adaptation{headers: h}
	var err error
	if spec.Type == "anthropic" {
		h.Set("X-Api-Key", e.UpstreamKey)
		if h.Get("Anthropic-Version") == "" {
			h.Set("Anthropic-Version", anthropicVersion)
		}
		if a.url, err = JoinURL(spec.BaseURL, "/v1/messages"); err != nil {
			return nil, err
		}
		if api == OpenAI {
			if out, err = openAIToAnthropic(out); err != nil {
				return nil, err
			}
			a.translateJSON = anthropicToOpenAI
			a.newStream = func() StreamTranslator { return newAnthropicStream() }
		}
	} else {
		h.Set("Authorization", "Bearer "+e.UpstreamKey)
		if a.url, err = JoinURL(spec.BaseURL, "/v1/chat/completions"); err != nil {
			return nil, err
		}
	}
	a.body, err = json.Marshal(out)
	return a, err
}
