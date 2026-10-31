package relay

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

// OpenAI Chat Completions ⇄ Anthropic Messages translation (non-streaming parts).
// Covers text, images, tools / tool results, system prompts and sampling params.

type obj = map[string]any

func openAIToAnthropic(body obj) (obj, error) {
	msgs, ok := body["messages"].([]any)
	if !ok {
		return nil, badRequest(`"messages" must be an array`)
	}

	var system []string
	var out []obj
	push := func(role string, blocks []any) {
		if len(blocks) == 0 {
			return
		}
		// Anthropic wants alternating turns; merge consecutive same-role messages.
		if n := len(out); n > 0 && out[n-1]["role"] == role {
			out[n-1]["content"] = append(out[n-1]["content"].([]any), blocks...)
			return
		}
		out = append(out, obj{"role": role, "content": blocks})
	}

	for _, raw := range msgs {
		m, _ := raw.(obj)
		switch m["role"] {
		case "system", "developer":
			system = append(system, textOf(m["content"]))
		case "user":
			push("user", contentBlocks(m["content"]))
		case "assistant":
			blocks := contentBlocks(m["content"])
			calls, _ := m["tool_calls"].([]any)
			for _, c := range calls {
				call, _ := c.(obj)
				fn, _ := call["function"].(obj)
				input, err := parseArgs(fn["arguments"])
				if err != nil {
					return nil, err
				}
				blocks = append(blocks, obj{"type": "tool_use", "id": call["id"], "name": fn["name"], "input": input})
			}
			push("assistant", blocks)
		case "tool":
			push("user", []any{obj{"type": "tool_result", "tool_use_id": m["tool_call_id"], "content": textOf(m["content"])}})
		}
	}

	req := obj{"model": body["model"], "messages": out, "max_tokens": firstOf(body["max_completion_tokens"], body["max_tokens"], anthropicDefaultMaxTokens)}
	if out == nil {
		req["messages"] = []obj{}
	}
	if len(system) > 0 {
		req["system"] = strings.Join(system, "\n\n")
	}
	for _, k := range []string{"temperature", "top_p", "stream"} {
		if v, ok := body[k]; ok {
			req[k] = v
		}
	}
	switch stop := body["stop"].(type) {
	case string:
		req["stop_sequences"] = []any{stop}
	case []any:
		req["stop_sequences"] = stop
	}
	if user, ok := body["user"].(string); ok {
		req["metadata"] = obj{"user_id": user}
	}
	if tools, ok := body["tools"].([]any); ok && len(tools) > 0 {
		conv := make([]any, 0, len(tools))
		for _, t := range tools {
			fn, _ := t.(obj)["function"].(obj)
			schema := fn["parameters"]
			if schema == nil {
				schema = obj{"type": "object", "properties": obj{}}
			}
			conv = append(conv, obj{"name": fn["name"], "description": fn["description"], "input_schema": schema})
		}
		req["tools"] = conv
		if choice := toolChoice(body["tool_choice"]); choice != nil {
			req["tool_choice"] = choice
		}
	}
	return req, nil
}

func anthropicToOpenAI(raw []byte, status int) ([]byte, error) {
	var b struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Model      string `json:"model"`
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type  string `json:"type"`
			Text  string `json:"text"`
			ID    string `json:"id"`
			Name  string `json:"name"`
			Input any    `json:"input"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		Error *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, err
	}
	if status >= 400 || b.Type == "error" {
		kind, msg := "upstream_error", "upstream error"
		if b.Error != nil {
			kind, msg = b.Error.Type, b.Error.Message
		}
		return json.Marshal(obj{"error": obj{"message": msg, "type": kind, "code": kind}})
	}

	var text strings.Builder
	var calls []any
	for _, c := range b.Content {
		switch c.Type {
		case "text":
			text.WriteString(c.Text)
		case "tool_use":
			input := c.Input
			if input == nil {
				input = obj{}
			}
			args, _ := json.Marshal(input)
			calls = append(calls, obj{"id": c.ID, "type": "function", "function": obj{"name": c.Name, "arguments": string(args)}})
		}
	}
	message := obj{"role": "assistant", "content": nil}
	if text.Len() > 0 {
		message["content"] = text.String()
	}
	if len(calls) > 0 {
		message["tool_calls"] = calls
	}
	return json.Marshal(obj{
		"id": b.ID, "object": "chat.completion", "created": time.Now().Unix(), "model": b.Model,
		"choices": []any{obj{"index": 0, "message": message, "finish_reason": finishReason(b.StopReason)}},
		"usage": obj{"prompt_tokens": b.Usage.InputTokens, "completion_tokens": b.Usage.OutputTokens,
			"total_tokens": b.Usage.InputTokens + b.Usage.OutputTokens},
	})
}

func finishReason(stop string) any {
	switch stop {
	case "end_turn", "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	case "refusal":
		return "content_filter"
	case "":
		return nil
	}
	return stop
}

var dataURL = regexp.MustCompile(`(?s)^data:([^;]+);base64,(.*)$`)

func contentBlocks(content any) []any {
	switch c := content.(type) {
	case string:
		if c == "" {
			return nil
		}
		return []any{obj{"type": "text", "text": c}}
	case []any:
		var blocks []any
		for _, p := range c {
			part, _ := p.(obj)
			switch part["type"] {
			case "text":
				blocks = append(blocks, obj{"type": "text", "text": part["text"]})
			case "image_url":
				img, _ := part["image_url"].(obj)
				url, _ := img["url"].(string)
				if m := dataURL.FindStringSubmatch(url); m != nil {
					blocks = append(blocks, obj{"type": "image", "source": obj{"type": "base64", "media_type": m[1], "data": m[2]}})
				} else {
					blocks = append(blocks, obj{"type": "image", "source": obj{"type": "url", "url": url}})
				}
			}
		}
		return blocks
	}
	return nil
}

func textOf(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case []any:
		var parts []string
		for _, p := range c {
			if part, _ := p.(obj); part["type"] == "text" {
				s, _ := part["text"].(string)
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func parseArgs(args any) (any, error) {
	s, _ := args.(string)
	if s == "" {
		return obj{}, nil
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, badRequest("tool_calls[].function.arguments is not valid JSON")
	}
	return v, nil
}

func toolChoice(choice any) obj {
	switch c := choice.(type) {
	case string:
		switch c {
		case "auto":
			return obj{"type": "auto"}
		case "required":
			return obj{"type": "any"}
		case "none":
			return obj{"type": "none"}
		}
	case obj:
		if fn, _ := c["function"].(obj); fn["name"] != nil {
			return obj{"type": "tool", "name": fn["name"]}
		}
	}
	return nil
}

func firstOf(vals ...any) any {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}
