package relay

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOpenAIRequestToAnthropic(t *testing.T) {
	var body obj
	_ = json.Unmarshal([]byte(`{
	  "model":"claude-x","max_tokens":100,"temperature":0.2,"stop":"END","stream":true,
	  "messages":[
	    {"role":"system","content":"be brief"},
	    {"role":"user","content":[{"type":"text","text":"what is this?"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}]},
	    {"role":"assistant","content":null,"tool_calls":[{"id":"t1","type":"function","function":{"name":"look","arguments":"{\"q\":1}"}}]},
	    {"role":"tool","tool_call_id":"t1","content":"a cat"},
	    {"role":"user","content":"thanks"}
	  ],
	  "tools":[{"type":"function","function":{"name":"look","description":"d","parameters":{"type":"object"}}}],
	  "tool_choice":"required"}`), &body)

	out, err := openAIToAnthropic(body)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(out)
	var got struct {
		System        string `json:"system"`
		MaxTokens     float64 `json:"max_tokens"`
		StopSequences []string `json:"stop_sequences"`
		ToolChoice    map[string]string `json:"tool_choice"`
		Tools         []struct{ InputSchema map[string]any `json:"input_schema"` } `json:"tools"`
		Messages      []struct {
			Role    string           `json:"role"`
			Content []map[string]any `json:"content"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(b, &got)

	if got.System != "be brief" || got.MaxTokens != 100 || got.StopSequences[0] != "END" || got.ToolChoice["type"] != "any" {
		t.Fatalf("params wrong: %s", b)
	}
	if got.Tools[0].InputSchema["type"] != "object" {
		t.Fatal("tool schema not converted")
	}
	if len(got.Messages) != 3 {
		t.Fatalf("want 3 turns (tool result merged with next user turn), got %d", len(got.Messages))
	}
	img := got.Messages[0].Content[1]["source"].(map[string]any)
	if img["type"] != "base64" || img["media_type"] != "image/png" || img["data"] != "AAAA" {
		t.Fatalf("image not converted: %v", img)
	}
	if use := got.Messages[1].Content[0]; use["type"] != "tool_use" || use["name"] != "look" {
		t.Fatalf("tool call not converted: %v", use)
	}
	if got.Messages[2].Content[0]["type"] != "tool_result" || got.Messages[2].Content[1]["text"] != "thanks" {
		t.Fatalf("tool result turn wrong: %v", got.Messages[2].Content)
	}
}

func TestAnthropicResponseToOpenAI(t *testing.T) {
	raw := `{"id":"msg_1","model":"claude-x","stop_reason":"tool_use",
	  "content":[{"type":"text","text":"hi"},{"type":"tool_use","id":"t1","name":"look","input":{"q":1}}],
	  "usage":{"input_tokens":5,"output_tokens":3}}`
	out, err := anthropicToOpenAI([]byte(raw), 200)
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		Object  string `json:"object"`
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct{ Function struct{ Arguments string } } `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage map[string]int `json:"usage"`
	}
	_ = json.Unmarshal(out, &r)
	c := r.Choices[0]
	if r.Object != "chat.completion" || c.Message.Content != "hi" || c.FinishReason != "tool_calls" ||
		c.Message.ToolCalls[0].Function.Arguments != `{"q":1}` || r.Usage["total_tokens"] != 8 {
		t.Fatalf("bad translation: %s", out)
	}

	errOut, _ := anthropicToOpenAI([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`), 529)
	if !strings.Contains(string(errOut), `"type":"overloaded_error"`) || !strings.Contains(string(errOut), `"message":"busy"`) {
		t.Fatalf("bad error translation: %s", errOut)
	}
}

func TestAnthropicStream(t *testing.T) {
	events := []string{
		`{"type":"message_start","message":{"id":"msg_1","model":"claude-x","usage":{"input_tokens":5}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hel"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lo"}}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"t1","name":"look"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"q\":"}}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
		`{"type":"message_stop"}`,
	}
	var raw strings.Builder
	for _, e := range events {
		raw.WriteString("event: x\ndata: " + e + "\n\n")
	}
	s := newAnthropicStream()
	var out strings.Builder
	in := raw.String()
	for i := 0; i < len(in); i += 7 { // awkward chunk boundaries
		out.WriteString(s.Push(in[i:min(i+7, len(in))]))
	}
	out.WriteString(s.End())

	chunks := strings.Split(strings.TrimSpace(out.String()), "\n\n")
	if chunks[len(chunks)-1] != "data: [DONE]" || strings.Count(out.String(), "[DONE]") != 1 {
		t.Fatalf("stream must end with exactly one [DONE]: %q", out.String())
	}
	var text strings.Builder
	var sawTool bool
	var lastFinish any
	for _, c := range chunks[:len(chunks)-1] {
		var ch struct {
			ID      string `json:"id"`
			Choices []struct {
				Delta        map[string]any `json:"delta"`
				FinishReason any            `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(c, "data: ")), &ch); err != nil || ch.ID != "msg_1" {
			t.Fatalf("bad chunk %q", c)
		}
		if s, ok := ch.Choices[0].Delta["content"].(string); ok {
			text.WriteString(s)
		}
		if calls, ok := ch.Choices[0].Delta["tool_calls"].([]any); ok && calls[0].(map[string]any)["id"] == "t1" {
			sawTool = true
		}
		lastFinish = ch.Choices[0].FinishReason
	}
	if text.String() != "Hello" || !sawTool || lastFinish != "stop" {
		t.Fatalf("text=%q tool=%v finish=%v", text.String(), sawTool, lastFinish)
	}
}
