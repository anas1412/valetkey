package relay

import (
	"encoding/json"
	"strings"
)

// Usage extracts token counts from OpenAI and Anthropic responses, buffered or SSE.
// Counts are cumulative, so the largest value seen wins.
type Usage struct {
	Input, Output int
	partial       string
}

type usageFields struct {
	PromptTokens     *int `json:"prompt_tokens"`
	CompletionTokens *int `json:"completion_tokens"`
	InputTokens      *int `json:"input_tokens"`
	OutputTokens     *int `json:"output_tokens"`
}

type usageCarrier struct {
	Usage   *usageFields `json:"usage"`
	Message *struct {
		Usage *usageFields `json:"usage"`
	} `json:"message"`
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
	for _, f := range []*usageFields{c.Usage, msgUsage(c)} {
		if f == nil {
			continue
		}
		if v := first(f.PromptTokens, f.InputTokens); v > u.Input {
			u.Input = v
		}
		if v := first(f.CompletionTokens, f.OutputTokens); v > u.Output {
			u.Output = v
		}
	}
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
