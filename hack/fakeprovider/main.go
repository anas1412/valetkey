// Command fakeprovider is a stand-in LLM provider and traffic generator for hack/demo.sh.
//
//	fakeprovider serve   answers OpenAI (/v1/chat/completions, /v1/models) and Anthropic (/v1/messages) requests
//	fakeprovider load    sends requests through Valetkey using VALETKEY_ENDPOINT and VALETKEY_KEY
//
// It never calls a real model: replies are canned and token counts are made up.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	mode := "serve"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	switch mode {
	case "serve":
		serve()
	case "load":
		load()
	default:
		log.Fatalf("unknown mode %q (serve or load)", mode)
	}
}

func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func envFloat(name string, def float64) float64 {
	if f, err := strconv.ParseFloat(os.Getenv(name), 64); err == nil {
		return f
	}
	return def
}

// serve: MODELS (comma-separated), FAIL_RATE (0..1, share of 500 replies), PORT.
func serve() {
	models := strings.Split(env("MODELS", "gpt-4o,gpt-4o-mini"), ",")
	failRate := envFloat("FAIL_RATE", 0.02)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		data := make([]map[string]any, len(models))
		for i, m := range models {
			data[i] = map[string]any{"id": m, "object": "model", "owned_by": "fakeprovider"}
		}
		writeJSON(w, 200, map[string]any{"object": "list", "data": data})
	})
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		if !decode(w, r, &req) || !think(w, failRate) {
			return
		}
		in, out := 20+rand.IntN(400), 10+rand.IntN(300)
		usage := openAIUsage(in, out)
		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			for _, word := range strings.Fields(reply) {
				fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]any{"object": "chat.completion.chunk", "model": req.Model,
					"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": word + " "}}}}))
			}
			fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", mustJSON(map[string]any{"object": "chat.completion.chunk", "model": req.Model,
				"choices": []any{}, "usage": usage}))
			return
		}
		writeJSON(w, 200, map[string]any{
			"id": fmt.Sprintf("chatcmpl-%d", rand.Int64()), "object": "chat.completion", "created": time.Now().Unix(), "model": req.Model,
			"choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": reply}}},
			"usage":   usage,
		})
	})
	mux.HandleFunc("POST /v1/messages", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		if !decode(w, r, &req) || !think(w, failRate) {
			return
		}
		writeJSON(w, 200, map[string]any{
			"id": fmt.Sprintf("msg_%d", rand.Int64()), "type": "message", "role": "assistant", "model": req.Model,
			"content": []any{map[string]any{"type": "text", "text": reply}}, "stop_reason": "end_turn",
			"usage": map[string]int{"input_tokens": 20 + rand.IntN(400), "output_tokens": 10 + rand.IntN(300),
				"cache_read_input_tokens": rand.IntN(2) * rand.IntN(800), "cache_creation_input_tokens": rand.IntN(4) / 3 * rand.IntN(400)},
		})
	})

	addr := ":" + env("PORT", "8080")
	log.Printf("fake provider on %s, models %v", addr, models)
	log.Fatal(http.ListenAndServe(addr, mux))
}

// openAIUsage reports usage like OpenAI and DeepSeek do: part of the prompt from the
// provider's cache, part of the output spent on reasoning.
func openAIUsage(in, out int) map[string]any {
	cached := rand.IntN(2) * rand.IntN(in)
	return map[string]any{
		"prompt_tokens": in, "completion_tokens": out, "total_tokens": in + out,
		"prompt_tokens_details":     map[string]int{"cached_tokens": cached},
		"completion_tokens_details": map[string]int{"reasoning_tokens": rand.IntN(2) * rand.IntN(out)},
		"prompt_cache_hit_tokens":   cached, "prompt_cache_miss_tokens": in - cached,
	}
}

const reply = "This is a canned reply from the Valetkey demo provider."

// think waits like a model would and sometimes fails, so the dashboards show latency and errors.
func think(w http.ResponseWriter, failRate float64) bool {
	time.Sleep(time.Duration(100+rand.IntN(900)) * time.Millisecond)
	if rand.Float64() < failRate {
		writeJSON(w, 500, map[string]any{"error": map[string]any{"message": "fake upstream error", "type": "server_error"}})
		return false
	}
	return true
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeJSON(w, 400, map[string]any{"error": map[string]any{"message": err.Error(), "type": "invalid_request_error"}})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// load: VALETKEY_ENDPOINT, VALETKEY_KEY (from the key's Secret), MODELS (models to ask for;
// a model the key may not use shows up as a 403), INTERVAL (average seconds between requests).
func load() {
	endpoint := strings.TrimSuffix(os.Getenv("VALETKEY_ENDPOINT"), "/")
	key := os.Getenv("VALETKEY_KEY")
	if endpoint == "" || key == "" {
		log.Fatal("VALETKEY_ENDPOINT and VALETKEY_KEY must be set")
	}
	models := strings.Split(env("MODELS", "gpt-4o-mini"), ",")
	interval := envFloat("INTERVAL", 2)
	maxTokens := int(envFloat("MAX_TOKENS", 512))
	client := &http.Client{Timeout: 30 * time.Second}

	for {
		model := models[rand.IntN(len(models))]
		body := mustJSON(map[string]any{"model": model, "max_tokens": maxTokens, "messages": []any{map[string]any{"role": "user", "content": "Summarize invoice " + strconv.Itoa(rand.IntN(10000))}}})
		req, _ := http.NewRequest("POST", endpoint+"/v1/chat/completions", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		start := time.Now()
		if res, err := client.Do(req); err != nil {
			log.Printf("%s: %v", model, err)
		} else {
			res.Body.Close()
			log.Printf("%s: %d in %s", model, res.StatusCode, time.Since(start).Round(time.Millisecond))
		}
		time.Sleep(time.Duration(rand.Float64() * 2 * interval * float64(time.Second)))
	}
}
