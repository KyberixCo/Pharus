package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/KyberixCo/Pharus/internal/config"
	"github.com/KyberixCo/Pharus/internal/llm/types"
	"github.com/KyberixCo/Pharus/pkg/logger"
)

type Client struct {
	cfg        *config.Config
	httpClient *http.Client
}

func NewClient(cfg *config.Config) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

type chatCompletionRequest struct {
	Model       string          `json:"model"`
	Messages    []types.Message `json:"messages"`
	Temperature float64         `json:"temperature"`
}

type choice struct {
	Message types.Message `json:"message"`
}

type chatCompletionResponse struct {
	Choices []choice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *Client) GenerateCompletion(ctx context.Context, messages []types.Message, temperature float64) (string, error) {
	if c.cfg == nil || c.cfg.OpenAI.APIKey == "" {
		return "", fmt.Errorf("OpenAI API key is missing. Set PHARUS_OPENAI_API_KEY env or configure in config.yaml")
	}

	modelName := c.cfg.OpenAI.Model
	if modelName == "" {
		modelName = "gpt-4o-mini"
	}

	baseURL := c.cfg.OpenAI.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	reqPayload := chatCompletionRequest{
		Model:       modelName,
		Messages:    messages,
		Temperature: temperature,
	}

	data, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal openai request: %w", err)
	}

	url := baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(data))
	if err != nil {
		return "", fmt.Errorf("failed to create openai http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.OpenAI.APIKey)

	log := logger.Get()
	log.Debug("Sending request to OpenAI LLM", "url", url, "model", modelName)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("OpenAI HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read OpenAI response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, string(body))
	}

	var chatRes chatCompletionResponse
	if err := json.Unmarshal(body, &chatRes); err != nil {
		return "", fmt.Errorf("failed to parse OpenAI JSON response: %s (%w)", string(body), err)
	}

	if chatRes.Error != nil && chatRes.Error.Message != "" {
		return "", fmt.Errorf("OpenAI API internal error: %s", chatRes.Error.Message)
	}

	if len(chatRes.Choices) == 0 {
		return "", fmt.Errorf("OpenAI returned zero choices in response")
	}

	return chatRes.Choices[0].Message.Content, nil
}
