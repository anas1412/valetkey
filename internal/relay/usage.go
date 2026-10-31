package relay

import (
	"encoding/json"
	"strings"
)

// Usage extracts token counts from OpenAI, DeepSeek and Anthropic responses, buffered
// or SSE, split the way providers bill them. Counts are cumulative, so the largest
// value seen for each field wins.
type Usage struct {
	Input        int // input tokens billed at the normal input price (no cache hit or write)
	CacheRead    int // input tokens served from the provider's prompt cache
	CacheWrite5m int // input tokens written to the cache (Anthropic 5 minute TTL)
	CacheWrite1h int // input tokens written to the cache (Anthropic 1 hour TTL)
	Output       int // all output tokens, reasoning included
	Reasoning    int // the reasoning share of Output, when the provider reports it
	AudioInput   int // the audio share of input (OpenAI audio models)
	AudioOutput  int // the audio share of Output
	partial      string
}

// InputTotal is every input token, whatever its price.
func (u *Usage) InputTotal() int { return u.Input + u.CacheRead + u.CacheWrite5m + u.CacheWrite1h }

type usageFields struct {
	// OpenAI and OpenAI-compatible (prompt_tokens includes cached tokens)
	PromptTokens        *int `json:"prompt_tokens"`
	CompletionTokens    *int `json:"completion_tokens"`
	PromptTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
		AudioTokens  int `json:"audio_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
		AudioTokens     int `json:"audio_tokens"`
	} `json:"completion_tokens_details"`
	// DeepSeek
	PromptCacheHitTokens  *int `json:"prompt_cache_hit_tokens"`
	PromptCacheMissTokens *int `json:"prompt_cache_miss_tokens"`
	// Responses API (input_tokens includes cached tokens, like prompt_tokens)
	InputTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
	// Anthropic (input_tokens excludes cache reads and writes)
	InputTokens              *int `json:"input_tokens"`
	OutputTokens             *int `json:"output_tokens"`
	CacheReadInputTokens     int  `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int  `json:"cache_creation_input_tokens"`
	CacheCreation            *struct {
		Ephemeral5m int `json:"ephemeral_5m_input_tokens"`
		Ephemeral1h int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
}

type usageCarrier struct {
	Usage   *usageFields `json:"usage"`
	Message *struct {    // Anthropic message_start
		Usage *usageFields `json:"usage"`
	} `json:"message"`
	Response *struct { // Responses API response.completed
		Usage *usageFields `json:"usage"`
	} `json:"response"`
}

// FeedJSON reads a complete (non-streaming) response body.
func (u *Usage) FeedJSON(body []byte) { u.read(body) }

// FeedSSE reads a raw SSE chunk; lines may be split across chunks.
func (u *Usage) FeedSSE(chunk string) {
	lines := strings.Split(u.partial+chunk, "\n")
	u.partial = lines[len(lines)-1]
	for _, l := range lines[:len(lines)-1] {
		u.readLine(l)
	}
}

// EndSSE flushes the trailing partial line.
func (u *Usage) EndSSE() {
	u.readLine(u.partial)
	u.partial = ""
}

func (u *Usage) readLine(line string) {
	data, ok := strings.CutPrefix(line, "data:")
	if !ok {
		return
	}
	data = strings.TrimSpace(data)
	if data == "" || data == "[DONE]" {
		return
	}
	u.read([]byte(data))
}

func (u *Usage) read(b []byte) {
	var c usageCarrier
	if json.Unmarshal(b, &c) != nil {
		return
	}
	for _, f := range []*usageFields{c.Usage, msgUsage(c), respUsage(c)} {
		if f != nil {
			u.merge(f)
		}
	}
}

func (u *Usage) merge(f *usageFields) {
	var n Usage
	switch {
	case f.PromptTokens != nil:
		cached := 0
		if f.PromptTokensDetails != nil {
			cached = f.PromptTokensDetails.CachedTokens
			n.AudioInput = f.PromptTokensDetails.AudioTokens
		}
		if f.PromptCacheHitTokens != nil {
			cached = *f.PromptCacheHitTokens
		}
		n.CacheRead = cached
		n.Input = max(*f.PromptTokens-cached, 0)
		if f.PromptCacheMissTokens != nil {
			n.Input = *f.PromptCacheMissTokens
		}
	case f.InputTokensDetails != nil && f.InputTokens != nil:
		n.CacheRead = f.InputTokensDetails.CachedTokens
		n.Input = max(*f.InputTokens-n.CacheRead, 0)
	default:
		if f.InputTokens != nil {
			n.Input = *f.InputTokens
		}
		n.CacheRead = f.CacheReadInputTokens
		if f.CacheCreation != nil && f.CacheCreation.Ephemeral5m+f.CacheCreation.Ephemeral1h > 0 {
			n.CacheWrite5m, n.CacheWrite1h = f.CacheCreation.Ephemeral5m, f.CacheCreation.Ephemeral1h
		} else {
			n.CacheWrite5m = f.CacheCreationInputTokens
		}
	}
	n.Output = first(f.CompletionTokens, f.OutputTokens)
	if f.CompletionTokensDetails != nil {
		n.Reasoning = f.CompletionTokensDetails.ReasoningTokens
		n.AudioOutput = f.CompletionTokensDetails.AudioTokens
	}
	if f.OutputTokensDetails != nil {
		n.Reasoning = f.OutputTokensDetails.ReasoningTokens
	}
	u.Input = max(u.Input, n.Input)
	u.CacheRead = max(u.CacheRead, n.CacheRead)
	u.CacheWrite5m = max(u.CacheWrite5m, n.CacheWrite5m)
	u.CacheWrite1h = max(u.CacheWrite1h, n.CacheWrite1h)
	u.Output = max(u.Output, n.Output)
	u.Reasoning = max(u.Reasoning, n.Reasoning)
	u.AudioInput = max(u.AudioInput, n.AudioInput)
	u.AudioOutput = max(u.AudioOutput, n.AudioOutput)
}

func respUsage(c usageCarrier) *usageFields {
	if c.Response == nil {
		return nil
	}
	return c.Response.Usage
}

func msgUsage(c usageCarrier) *usageFields {
	if c.Message == nil {
		return nil
	}
	return c.Message.Usage
}

func first(a, b *int) int {
	if a != nil {
		return *a
	}
	if b != nil {
		return *b
	}
	return 0
}
