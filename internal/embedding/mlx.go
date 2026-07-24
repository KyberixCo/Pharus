package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/philippgille/chromem-go"
)

type MLXProvider struct {
	baseURL string
	model   string
	client  *http.Client
}

type mlxEmbedReq struct {
	Input string `json:"input"`
	Model string `json:"model,omitempty"`
}

type mlxEmbedRes struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Embedding []float64 `json:"embedding,omitempty"`
}

func NewMLXProvider(baseURL string, model string) *MLXProvider {
	if baseURL == "" {
		baseURL = "http://localhost:8000"
	}
	if model == "" {
		model = "bge-small-en-v1.5"
	}
	return &MLXProvider{
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *MLXProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody, _ := json.Marshal(mlxEmbedReq{
		Input: text,
		Model: p.model,
	})

	url := p.baseURL + "/v1/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request for MLX server at %s: %w", url, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("MLX server unreachable at %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MLX server returned status %d", resp.StatusCode)
	}

	var res mlxEmbedRes
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode response from MLX server: %w", err)
	}

	var vec []float64
	if len(res.Data) > 0 && len(res.Data[0].Embedding) > 0 {
		vec = res.Data[0].Embedding
	} else if len(res.Embedding) > 0 {
		vec = res.Embedding
	} else {
		return nil, fmt.Errorf("MLX server returned empty embedding vector")
	}

	result := make([]float32, len(vec))
	for i, v := range vec {
		result[i] = float32(v)
	}
	return result, nil
}

func (p *MLXProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	results := make([][]float32, len(texts))
	for i, text := range texts {
		vec, err := p.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		results[i] = vec
	}
	return results, nil
}

func (p *MLXProvider) ChromemEmbeddingFunc() chromem.EmbeddingFunc {
	return func(ctx context.Context, text string) ([]float32, error) {
		vec, err := p.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		return vec, nil
	}
}
