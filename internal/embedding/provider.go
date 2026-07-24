package embedding

import (
	"context"
	"strings"

	"github.com/KyberixCo/Pharus/internal/config"
	"github.com/philippgille/chromem-go"
)

type Provider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	ChromemEmbeddingFunc() chromem.EmbeddingFunc
}

func NewProvider(cfg *config.Config) Provider {
	provider := strings.ToLower(cfg.Embed.Provider)
	switch provider {
	case "omlx":
		return NewOMLXProvider(cfg.Embed.URL, cfg.Embed.Model)
	case "mlx":
		return NewMLXProvider(cfg.Embed.URL, cfg.Embed.Model)
	case "openai", "external", "custom":
		return NewExternalProvider(cfg.Embed.URL, cfg.Embed.Model, cfg.Embed.APIKey)
	default: // "ollama"
		return NewOllamaProvider(cfg.Embed.URL, cfg.Embed.Model)
	}
}
