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

type ExternalProvider struct {
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
}

func NewExternalProvider(baseURL, model, apiKey string) *ExternalProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "text-embedding-3-small"
	}
	return &ExternalProvider{
		baseURL: baseURL,
		model:   model,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (p *ExternalProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody, _ := json.Marshal(mlxEmbedReq{
		Input: text,
		Model: p.model,
	})

	url := p.baseURL + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create external embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("external embedding request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("external embedding server returned status %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	var res mlxEmbedRes
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode external embedding response: %w", err)
	}

	if len(res.Data) == 0 || len(res.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("external embedding server returned empty embedding vector")
	}

	result := make([]float32, len(res.Data[0].Embedding))
	for i, v := range res.Data[0].Embedding {
		result[i] = float32(v)
	}
	return result, nil
}

func (p *ExternalProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
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

func (p *ExternalProvider) ChromemEmbeddingFunc() chromem.EmbeddingFunc {
	return func(ctx context.Context, text string) ([]float32, error) {
		return p.Embed(ctx, text)
	}
}
