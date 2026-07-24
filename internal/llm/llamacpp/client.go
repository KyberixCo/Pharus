package llamacpp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/KyberixCo/Pharus/internal/config"
	"github.com/KyberixCo/Pharus/internal/llm/gbnf"
	"github.com/KyberixCo/Pharus/pkg/logger"
)

// Client is a Go client for local llama.cpp server with GBNF grammar sampling support.
type Client struct {
	cfg        *config.Config
	httpClient *http.Client
	gbnfGen    *gbnf.GBNFGenerator
}

// NewClient initializes a new llama.cpp local inference client.
func NewClient(cfg *config.Config) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		gbnfGen: gbnf.NewGBNFGenerator(),
	}
}

// GBNFGenerator returns the underlying GBNF generator instance.
func (c *Client) GBNFGenerator() *gbnf.GBNFGenerator {
	return c.gbnfGen
}

// GenerateCompletion calls llama.cpp /completion endpoint with optional GBNF grammar string.
func (c *Client) GenerateCompletion(ctx context.Context, prompt string, grammar string) (string, error) {
	baseURL := c.cfg.LlamaCPP.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	reqPayload := CompletionRequest{
		Prompt:      prompt,
		Grammar:     grammar,
		Temperature: c.cfg.LlamaCPP.Temperature,
		TopK:        c.cfg.LlamaCPP.TopK,
		TopP:        c.cfg.LlamaCPP.TopP,
		NPredict:    2048,
	}

	data, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal completion request: %w", err)
	}

	url := baseURL + "/completion"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(data))
	if err != nil {
		return "", fmt.Errorf("failed to create llama.cpp HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	log := logger.Get()
	log.Debug("Sending request to llama.cpp", "url", url, "grammar_enabled", grammar != "")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llama.cpp HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read llama.cpp response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llama.cpp API error (status %d): %s", resp.StatusCode, string(body))
	}

	var compRes CompletionResponse
	if err := json.Unmarshal(body, &compRes); err != nil {
		return "", fmt.Errorf("failed to parse llama.cpp response: %s (%w)", string(body), err)
	}

	return compRes.Content, nil
}

// GenerateChatCompletion calls llama.cpp /v1/chat/completions endpoint with optional GBNF grammar.
func (c *Client) GenerateChatCompletion(ctx context.Context, messages []ChatMessage, grammar string) (string, error) {
	return c.generateChatCompletion(ctx, messages, grammar, nil)
}

func (c *Client) generateChatCompletion(ctx context.Context, messages []ChatMessage, grammar string, temperature *float64) (string, error) {
	baseURL := c.cfg.LlamaCPP.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	reqPayload := ChatCompletionRequest{
		Model:       c.cfg.LlamaCPP.Model,
		Messages:    messages,
		Grammar:     grammar,
		Temperature: c.cfg.LlamaCPP.Temperature,
		TopK:        c.cfg.LlamaCPP.TopK,
		TopP:        c.cfg.LlamaCPP.TopP,
		MaxTokens:   2048,
	}
	if temperature != nil {
		reqPayload.Temperature = *temperature
	}

	data, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal chat completion request: %w", err)
	}

	url := baseURL + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(data))
	if err != nil {
		return "", fmt.Errorf("failed to create chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llama.cpp chat HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read llama.cpp chat response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llama.cpp chat API error (status %d): %s", resp.StatusCode, string(body))
	}

	var chatRes ChatCompletionResponse
	if err := json.Unmarshal(body, &chatRes); err != nil {
		return "", fmt.Errorf("failed to parse llama.cpp chat response: %s (%w)", string(body), err)
	}

	if len(chatRes.Choices) == 0 {
		return "", fmt.Errorf("llama.cpp returned zero choices")
	}

	return chatRes.Choices[0].Message.Content, nil
}

// GenerateStructuredOutput uses GBNF grammar to guarantee 100% syntactically valid JSON output for given fields.
func (c *Client) GenerateStructuredOutput(ctx context.Context, prompt string, fields map[string]string) (string, error) {
	grammar := c.gbnfGen.GenerateObjectGrammar(fields)
	return c.GenerateCompletion(ctx, prompt, grammar)
}

// GenerateToolCall uses GBNF grammar to guarantee a valid MCP tool call payload {"tool": ..., "arguments": ...}.
func (c *Client) GenerateToolCall(ctx context.Context, toolName string, args map[string]string) (string, error) {
	grammar := c.gbnfGen.GenerateToolCallGrammar(toolName, args)
	prompt := fmt.Sprintf("Execute tool '%s' with appropriate arguments based on request.", toolName)
	return c.GenerateCompletion(ctx, prompt, grammar)
}

// MaskLogits applies deterministic GBNF logit masking on vocabulary logits slice.
func (c *Client) MaskLogits(logits []float32, validIndices []int) []float32 {
	return c.gbnfGen.MaskLogits(logits, validIndices)
}
