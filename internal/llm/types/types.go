package types

import (
	"context"
)

type Message struct {
	Role    string `json:"role"`    // "system", "user", "assistant"
	Content string `json:"content"`
}

type Provider interface {
	GenerateCompletion(ctx context.Context, messages []Message, temperature float64) (string, error)
}
