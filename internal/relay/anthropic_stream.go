package relay

import (
	"encoding/json"
	"strings"
	"time"
)

// anthropicStream translates an Anthropic Messages SSE stream into OpenAI
// chat.completion.chunk SSE. Anthropic sends one JSON object per data: line.
type anthropicStream struct {
	partial   string
	id, model string
	created   int64
	toolIndex map[int]int // Anthropic content block index → OpenAI tool_calls index
	done      bool
}

func newAnthropicStream() *anthropicStream {
	return &anthropicStream{created: time.Now().Unix(), toolIndex: map[int]int{}}
}

func (s *anthropicStream) Push(chunk string) string {
	lines := strings.Split(s.partial+chunk, "\n")
	s.partial = lines[len(lines)-1]
	var out strings.Builder
	for _, l := range lines[:len(lines)-1] {
		out.WriteString(s.line(l))
	}
	return out.String()
}

func (s *anthropicStream) End() string {
	out := s.line(s.partial)
	s.partial = ""
	if !s.done {
		out += "data: [DONE]\n\n"
	}
	return out
}

type anthropicEvent struct {
	Type    string `json:"type"`
	Index   int    `json:"index"`
	Message struct {
		ID    string `json:"id"`
		Model string `json:"model"`
	} `json:"message"`
	ContentBlock struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (s *anthropicStream) line(line string) string {
	data, ok := strings.CutPrefix(line, "data:")
	if !ok {
		return ""
	}
	var ev anthropicEvent
	if json.Unmarshal([]byte(strings.TrimSpace(data)), &ev) != nil {
		return ""
	}
	switch ev.Type {
	case "message_start":
		s.id, s.model = ev.Message.ID, ev.Message.Model
		return s.chunk(obj{"role": "assistant", "content": ""}, nil)
	case "content_block_start":
		if ev.ContentBlock.Type == "tool_use" {
			idx := len(s.toolIndex)
			s.toolIndex[ev.Index] = idx
			return s.chunk(obj{"tool_calls": []any{obj{
				"index": idx, "id": ev.ContentBlock.ID, "type": "function",
				"function": obj{"name": ev.ContentBlock.Name, "arguments": ""},
			}}}, nil)
		}
	case "content_block_delta":
		switch ev.Delta.Type {
		case "text_delta":
			return s.chunk(obj{"content": ev.Delta.Text}, nil)
		case "input_json_delta":
			return s.chunk(obj{"tool_calls": []any{obj{"index": s.toolIndex[ev.Index], "function": obj{"arguments": ev.Delta.PartialJSON}}}}, nil)
		}
	case "message_delta":
		return s.chunk(obj{}, finishReason(ev.Delta.StopReason))
	case "message_stop":
		s.done = true
		return "data: [DONE]\n\n"
	case "error":
		return sse(obj{"error": obj{"message": ev.Error.Message, "type": ev.Error.Type}})
	}
	return ""
}

func (s *anthropicStream) chunk(delta obj, finish any) string {
	return sse(obj{
		"id": s.id, "object": "chat.completion.chunk", "created": s.created, "model": s.model,
		"choices": []any{obj{"index": 0, "delta": delta, "finish_reason": finish}},
	})
}

func sse(v any) string {
	b, _ := json.Marshal(v)
	return "data: " + string(b) + "\n\n"
}
