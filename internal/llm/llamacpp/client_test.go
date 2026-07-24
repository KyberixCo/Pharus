package llamacpp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KyberixCo/Pharus/internal/config"
)

func TestLlamaCPPClient_GenerateCompletion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/completion" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req CompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		if req.Prompt != "Generate topic" {
			t.Errorf("expected prompt 'Generate topic', got '%s'", req.Prompt)
		}
		if req.Grammar == "" {
			t.Errorf("expected non-empty GBNF grammar in request")
		}

		res := CompletionResponse{
			Content: `{"topic": "Artificial Intelligence", "score": 95}`,
			Model:   "qwen2.5-coder-7b-instruct",
			Stop:    true,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	}))
	defer ts.Close()

	cfg := config.DefaultConfig()
	cfg.LlamaCPP.BaseURL = ts.URL

	client := NewClient(cfg)
	out, err := client.GenerateStructuredOutput(context.Background(), "Generate topic", map[string]string{
		"topic": "string",
		"score": "number",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Artificial Intelligence") {
		t.Errorf("expected output to contain 'Artificial Intelligence', got: %s", out)
	}
}

func TestLlamaCPPClient_GenerateToolCall(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req CompletionRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		if !strings.Contains(req.Grammar, "search_web") {
			t.Errorf("expected tool grammar to include 'search_web', got:\n%s", req.Grammar)
		}

		res := CompletionResponse{
			Content: `{"tool": "search_web", "arguments": {"query": "golang gbnf"}}`,
			Stop:    true,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	}))
	defer ts.Close()

	cfg := config.DefaultConfig()
	cfg.LlamaCPP.BaseURL = ts.URL

	client := NewClient(cfg)
	out, err := client.GenerateToolCall(context.Background(), "search_web", map[string]string{
		"query": "string",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "golang gbnf") {
		t.Errorf("expected output to contain 'golang gbnf', got: %s", out)
	}
}

func TestLlamaCPPClient_GenerateChatCompletion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected chat path: %s", r.URL.Path)
		}

		res := ChatCompletionResponse{
			Model: "qwen2.5-coder-7b-instruct",
			Choices: []struct {
				Index        int         `json:"index"`
				Message      ChatMessage `json:"message"`
				FinishReason string      `json:"finish_reason"`
			}{
				{
					Index: 0,
					Message: ChatMessage{
						Role:    "assistant",
						Content: "Hello from local llama.cpp!",
					},
					FinishReason: "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	}))
	defer ts.Close()

	cfg := config.DefaultConfig()
	cfg.LlamaCPP.BaseURL = ts.URL

	client := NewClient(cfg)
	out, err := client.GenerateChatCompletion(context.Background(), []ChatMessage{
		{Role: "user", Content: "Hello"},
	}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out != "Hello from local llama.cpp!" {
		t.Errorf("expected 'Hello from local llama.cpp!', got '%s'", out)
	}
}
