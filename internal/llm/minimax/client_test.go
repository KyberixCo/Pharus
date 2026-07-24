package minimax

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KyberixCo/Pharus/internal/config"
	"github.com/KyberixCo/Pharus/internal/llm/types"
)

func TestMiniMaxAnthropicMessagesAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/anthropic/v1/messages" {
			t.Errorf("expected path /anthropic/v1/messages, got %s", r.URL.Path)
		}
		if r.Header.Get("X-Api-Key") != "test-key" {
			t.Errorf("expected X-Api-Key test-key, got %s", r.Header.Get("X-Api-Key"))
		}
		if r.Header.Get("Anthropic-Version") != "2023-06-01" {
			t.Errorf("expected Anthropic-Version header, got %s", r.Header.Get("Anthropic-Version"))
		}

		var req MessagesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if req.Model != "MiniMax-M3" {
			t.Errorf("expected model MiniMax-M3, got %s", req.Model)
		}
		if req.System != "You are a helpful assistant" {
			t.Errorf("expected system prompt, got %q", req.System)
		}
		if req.MaxTokens != 16384 {
			t.Errorf("expected max_tokens 16384, got %d", req.MaxTokens)
		}
		if len(req.Messages) != 1 || req.Messages[0].Content[0].Text != "Hola" {
			t.Errorf("unexpected messages payload: %#v", req.Messages)
		}

		resp := MessagesResponse{
			ID:         "msg_123",
			Type:       "message",
			Role:       "assistant",
			Model:      "MiniMax-M3",
			StopReason: "end_turn",
			Content: []ContentBlock{
				{Type: "thinking", Thinking: "internal", Signature: "signature"},
				{Type: "text", Text: "Respuesta de prueba desde MiniMax-M3"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.MiniMax.APIKey = "test-key"
	cfg.MiniMax.BaseURL = server.URL + "/anthropic"

	reply, err := NewClient(cfg).GenerateCompletion(context.Background(), []types.Message{
		{Role: "system", Content: "You are a helpful assistant"},
		{Role: "user", Content: "Hola"},
	}, 0.2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply != "Respuesta de prueba desde MiniMax-M3" {
		t.Errorf("unexpected reply %q", reply)
	}
}

func TestMiniMaxRejectsTruncatedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(MessagesResponse{
			StopReason: "max_tokens",
			Content:    []ContentBlock{{Type: "text", Text: "partial report"}},
		})
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.MiniMax.APIKey = "test-key"
	cfg.MiniMax.BaseURL = server.URL + "/anthropic"
	cfg.MiniMax.MaxTokens = 8192

	_, err := NewClient(cfg).GenerateCompletion(context.Background(), []types.Message{{Role: "user", Content: "research"}}, 0.2)
	if err == nil || !strings.Contains(err.Error(), "truncated") || !strings.Contains(err.Error(), "8192") {
		t.Fatalf("expected explicit truncation error, got %v", err)
	}
}

func TestMiniMaxRejectsUnsupportedMaxTokens(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MiniMax.APIKey = "test-key"
	cfg.MiniMax.MaxTokens = maxMaxTokens + 1

	_, err := NewClient(cfg).GenerateCompletion(context.Background(), []types.Message{{Role: "user", Content: "hello"}}, 0.2)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected max_tokens validation error, got %v", err)
	}
}

func TestMiniMaxHonorsPerRequestMaxTokens(t *testing.T) {
	var received int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req MessagesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		received = req.MaxTokens
		_ = json.NewEncoder(w).Encode(MessagesResponse{
			StopReason: "end_turn",
			Content:    []ContentBlock{{Type: "text", Text: "bounded response"}},
		})
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.MiniMax.APIKey = "test-key"
	cfg.MiniMax.BaseURL = server.URL + "/anthropic"
	ctx := types.WithRequestOptions(context.Background(), types.RequestOptions{MaxTokens: 2048})
	if _, err := NewClient(cfg).GenerateCompletion(ctx, []types.Message{{Role: "user", Content: "research"}}, 0.2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received != 2048 {
		t.Fatalf("expected request max_tokens=2048, got %d", received)
	}
}

func TestResolveEndpointURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "https://api.minimax.io/anthropic/v1/messages"},
		{"https://api.minimax.io", "https://api.minimax.io/anthropic/v1/messages"},
		{"https://api.minimax.io/v1", "https://api.minimax.io/anthropic/v1/messages"},
		{"https://api.minimax.io/v1/responses", "https://api.minimax.io/anthropic/v1/messages"},
		{"https://api.minimax.io/anthropic", "https://api.minimax.io/anthropic/v1/messages"},
		{"https://api.minimax.io/anthropic/v1", "https://api.minimax.io/anthropic/v1/messages"},
		{"https://api.minimax.io/anthropic/v1/messages", "https://api.minimax.io/anthropic/v1/messages"},
		{"http://localhost:9000/v1", "http://localhost:9000/v1/messages"},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got, err := resolveEndpointURL(test.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.want {
				t.Errorf("resolveEndpointURL(%q) = %q; want %q", test.input, got, test.want)
			}
		})
	}

	_, err := resolveEndpointURL("://bad")
	if err == nil {
		t.Fatal("expected invalid URL error")
	}
}
