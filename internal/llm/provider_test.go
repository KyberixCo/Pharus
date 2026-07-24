package llm

import (
	"testing"

	"github.com/KyberixCo/Pharus/internal/config"
)

func TestNewProviderFactory(t *testing.T) {
	cfg := config.DefaultConfig()

	// Test default / minimax
	cfg.LLM.Provider = "minimax"
	p, err := NewProvider(cfg)
	if err != nil || p == nil {
		t.Fatalf("Expected valid minimax provider, got err: %v", err)
	}

	// Test openai
	cfg.LLM.Provider = "openai"
	p, err = NewProvider(cfg)
	if err != nil || p == nil {
		t.Fatalf("Expected valid openai provider, got err: %v", err)
	}

	// Test anthropic
	cfg.LLM.Provider = "anthropic"
	p, err = NewProvider(cfg)
	if err != nil || p == nil {
		t.Fatalf("Expected valid anthropic provider, got err: %v", err)
	}

	// Test ollama
	cfg.LLM.Provider = "ollama"
	p, err = NewProvider(cfg)
	if err != nil || p == nil {
		t.Fatalf("Expected valid ollama provider, got err: %v", err)
	}

	// Test llama.cpp as a first-class provider
	cfg.LLM.Provider = "llamacpp"
	p, err = NewProvider(cfg)
	if err != nil || p == nil {
		t.Fatalf("Expected valid llama.cpp provider, got err: %v", err)
	}

	// Test unsupported provider
	cfg.LLM.Provider = "unsupported_provider"
	_, err = NewProvider(cfg)
	if err == nil {
		t.Fatalf("Expected error for unsupported provider, got nil")
	}
}
