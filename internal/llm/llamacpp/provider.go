package llamacpp

import (
	"context"

	"github.com/KyberixCo/Pharus/internal/config"
	"github.com/KyberixCo/Pharus/internal/llm/types"
)

// Provider adapts the llama.cpp chat endpoint to the main LLM provider
// contract. GBNF-specific operations remain available from Client, while the
// complete research workflow can now select llama.cpp for normal generation.
type Provider struct {
	client *Client
}

func NewProvider(cfg *config.Config) *Provider {
	return &Provider{client: NewClient(cfg)}
}

func (p *Provider) GenerateCompletion(ctx context.Context, messages []types.Message, temperature float64) (string, error) {
	chatMessages := make([]ChatMessage, len(messages))
	for i, message := range messages {
		chatMessages[i] = ChatMessage{Role: message.Role, Content: message.Content}
	}
	return p.client.generateChatCompletion(ctx, chatMessages, "", &temperature)
}

var _ types.Provider = (*Provider)(nil)
