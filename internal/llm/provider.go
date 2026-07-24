package llm

import (
	"fmt"
	"strings"
	"time"

	"github.com/KyberixCo/Pharus/internal/config"
	"github.com/KyberixCo/Pharus/internal/llm/anthropic"
	"github.com/KyberixCo/Pharus/internal/llm/llamacpp"
	"github.com/KyberixCo/Pharus/internal/llm/minimax"
	"github.com/KyberixCo/Pharus/internal/llm/ollama"
	"github.com/KyberixCo/Pharus/internal/llm/openai"
	"github.com/KyberixCo/Pharus/internal/llm/types"
)

type Message = types.Message
type Provider = types.Provider

func NewProvider(cfg *config.Config) (Provider, error) {
	providerName := "minimax"
	if cfg != nil && cfg.LLM.Provider != "" {
		providerName = strings.ToLower(cfg.LLM.Provider)
	}

	var provider Provider
	switch providerName {
	case "openai":
		provider = openai.NewClient(cfg)
	case "anthropic":
		provider = anthropic.NewClient(cfg)
	case "ollama":
		provider = ollama.NewClient(cfg)
	case "llamacpp", "llama.cpp":
		provider = llamacpp.NewProvider(cfg)
	case "minimax":
		provider = minimax.NewClient(cfg)
	default:
		return nil, fmt.Errorf("unsupported LLM provider '%s'. Supported providers: minimax, openai, anthropic, ollama, llamacpp", providerName)
	}
	policy := RetryPolicy{}
	if cfg != nil {
		policy.MaxAttempts = cfg.LLM.RetryMaxAttempts
		policy.InitialWait = time.Duration(cfg.LLM.RetryInitialBackoffSeconds) * time.Second
		policy.MaxWait = time.Duration(cfg.LLM.RetryMaxBackoffSeconds) * time.Second
	}
	return NewRetryingProvider(provider, policy), nil
}
