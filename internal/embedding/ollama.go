package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/philippgille/chromem-go"
)

type OllamaProvider struct {
	baseURL string
	model   string
	client  *http.Client
}

type ollamaEmbedReq struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaEmbedRes struct {
	Embedding []float64 `json:"embedding"`
}

func NewOllamaProvider(baseURL string, model string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "nomic-embed-text"
	}
	return &OllamaProvider{
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *OllamaProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody, _ := json.Marshal(ollamaEmbedReq{
		Model:  p.model,
		Prompt: text,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/embeddings", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create Ollama embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Ollama embedding request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return nil, fmt.Errorf("Ollama embedding server returned status %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	var res ollamaEmbedRes
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode Ollama embedding response: %w", err)
	}
	if len(res.Embedding) == 0 {
		return nil, fmt.Errorf("Ollama embedding server returned empty embedding vector")
	}

	result := make([]float32, len(res.Embedding))
	for i, v := range res.Embedding {
		result[i] = float32(v)
	}
	return result, nil
}

// EmbedBatch processes multiple text prompts concurrently in batch.
func (p *OllamaProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	results := make([][]float32, len(texts))
	type jobResult struct {
		idx int
		vec []float32
		err error
	}

	jobs := make(chan int, len(texts))
	resChan := make(chan jobResult, len(texts))

	workers := 4
	if len(texts) < workers {
		workers = len(texts)
	}

	for w := 0; w < workers; w++ {
		go func() {
			for i := range jobs {
				vec, err := p.Embed(ctx, texts[i])
				resChan <- jobResult{idx: i, vec: vec, err: err}
			}
		}()
	}

	for i := range texts {
		jobs <- i
	}
	close(jobs)

	for i := 0; i < len(texts); i++ {
		res := <-resChan
		if res.err != nil {
			return nil, res.err
		}
		results[res.idx] = res.vec
	}

	return results, nil
}

func (p *OllamaProvider) ChromemEmbeddingFunc() chromem.EmbeddingFunc {
	return func(ctx context.Context, text string) ([]float32, error) {
		return p.Embed(ctx, text)
	}
}

// fallbackEmbed generates a 384-dimensional normalized vector deterministically from text.
func fallbackEmbed(text string) []float32 {
	const dim = 384
	vec := make([]float32, dim)
	var sumSq float64

	for i := 0; i < dim; i++ {
		val := float64(0.0)
		for j := 0; j < len(text); j++ {
			val += float64(text[j]) * math.Sin(float64(i+1)*float64(j+1))
		}
		vec[i] = float32(val)
		sumSq += val * val
	}

	norm := math.Sqrt(sumSq)
	if norm > 0 {
		for i := 0; i < dim; i++ {
			vec[i] /= float32(norm)
		}
	}
	return vec
}
