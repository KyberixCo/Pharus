package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/philippgille/chromem-go"
)

const (
	omlxEmbeddingChunkRunes = 8_000
	maxOmlxEmbeddingRunes   = 16_000
)

type OMLXProvider struct {
	baseURL string
	model   string
	client  *http.Client
	gate    chan struct{}
}

func NewOMLXProvider(baseURL string, model string) *OMLXProvider {
	if baseURL == "" {
		baseURL = "http://localhost:8000"
	}
	if model == "" {
		model = "mlx-community/Qwen3-Embedding-0.6B-8bit"
	}
	return &OMLXProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 2 * time.Minute},
		gate:    make(chan struct{}, 1),
	}
}

func (p *OMLXProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	select {
	case p.gate <- struct{}{}:
		defer func() { <-p.gate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	runes := []rune(text)
	if len(runes) > maxOmlxEmbeddingRunes {
		runes = runes[:maxOmlxEmbeddingRunes]
		text = string(runes)
	}
	if len(runes) <= omlxEmbeddingChunkRunes {
		return p.embedOne(ctx, text)
	}

	var combined []float64
	totalWeight := 0
	chunkCount := (len(runes) + omlxEmbeddingChunkRunes - 1) / omlxEmbeddingChunkRunes
	for start := 0; start < len(runes); start += omlxEmbeddingChunkRunes {
		end := min(start+omlxEmbeddingChunkRunes, len(runes))
		vec, err := p.embedOne(ctx, string(runes[start:end]))
		if err != nil {
			return nil, fmt.Errorf("failed to embed OMLX chunk %d/%d: %w", start/omlxEmbeddingChunkRunes+1, chunkCount, err)
		}
		if combined == nil {
			combined = make([]float64, len(vec))
		} else if len(vec) != len(combined) {
			return nil, fmt.Errorf("OMLX returned inconsistent embedding dimensions: %d and %d", len(combined), len(vec))
		}
		weight := end - start
		totalWeight += weight
		for i, value := range vec {
			combined[i] += float64(value) * float64(weight)
		}
	}

	result := make([]float32, len(combined))
	var normSquared float64
	for _, value := range combined {
		normSquared += value * value
	}
	norm := math.Sqrt(normSquared)
	if totalWeight == 0 || norm == 0 {
		return nil, fmt.Errorf("OMLX returned empty combined embedding vector")
	}
	for i, value := range combined {
		result[i] = float32(value / norm)
	}
	return result, nil
}

func (p *OMLXProvider) embedOne(ctx context.Context, text string) ([]float32, error) {
	reqBody, _ := json.Marshal(mlxEmbedReq{
		Input: text,
		Model: p.model,
	})

	url := p.baseURL + "/v1/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request for OMLX server at %s: %w", url, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OMLX server unreachable at %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		var apiErr struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &apiErr) == nil && strings.TrimSpace(apiErr.Error.Message) != "" {
			return nil, fmt.Errorf("OMLX server returned HTTP status %d at %s: %s", resp.StatusCode, url, apiErr.Error.Message)
		}
		return nil, fmt.Errorf("OMLX server returned HTTP status %d at %s", resp.StatusCode, url)
	}

	var res mlxEmbedRes
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode response from OMLX server: %w", err)
	}

	var vec []float64
	if len(res.Data) > 0 && len(res.Data[0].Embedding) > 0 {
		vec = res.Data[0].Embedding
	} else if len(res.Embedding) > 0 {
		vec = res.Embedding
	} else {
		return nil, fmt.Errorf("OMLX server returned empty embedding vector")
	}

	result := make([]float32, len(vec))
	for i, v := range vec {
		result[i] = float32(v)
	}
	return result, nil
}

func (p *OMLXProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
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

func (p *OMLXProvider) ChromemEmbeddingFunc() chromem.EmbeddingFunc {
	return func(ctx context.Context, text string) ([]float32, error) {
		vec, err := p.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		return vec, nil
	}
}
