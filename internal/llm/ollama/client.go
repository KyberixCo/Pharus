package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
			Timeout: 180 * time.Second, // local LLM inference can take a bit longer
		},
	}
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature"`
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []types.Message `json:"messages"`
	Stream   bool            `json:"stream"`
	Options  ollamaOptions   `json:"options"`
}

type ollamaChatResponse struct {
	Model   string        `json:"model"`
	Message types.Message `json:"message"`
	Done    bool          `json:"done"`
	Error   string        `json:"error,omitempty"`
}

func (c *Client) GenerateCompletion(ctx context.Context, messages []types.Message, temperature float64) (string, error) {
	baseURL := "http://localhost:11434"
	modelName := "llama3.1"

	if c.cfg != nil {
		if c.cfg.OllamaLLM.BaseURL != "" {
			baseURL = c.cfg.OllamaLLM.BaseURL
		}
		if c.cfg.OllamaLLM.Model != "" {
			modelName = c.cfg.OllamaLLM.Model
		}
	}

	reqPayload := ollamaChatRequest{
		Model:    modelName,
		Messages: messages,
		Stream:   false,
		Options: ollamaOptions{
			Temperature: temperature,
		},
	}

	data, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal ollama chat request: %w", err)
	}

	url := strings.TrimRight(baseURL, "/") + "/api/chat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(data))
	if err != nil {
		return "", fmt.Errorf("failed to create ollama http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	log := logger.Get()
	log.Debug("Sending request to local Ollama LLM", "url", url, "model", modelName)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Ollama HTTP request failed (is Ollama running at %s?): %w", baseURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read Ollama response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Ollama API error (status %d): %s", resp.StatusCode, string(body))
	}

	var chatRes ollamaChatResponse
	if err := json.Unmarshal(body, &chatRes); err != nil {
		return "", fmt.Errorf("failed to parse Ollama JSON response: %s (%w)", string(body), err)
	}

	if chatRes.Error != "" {
		return "", fmt.Errorf("Ollama internal error: %s", chatRes.Error)
	}

	return chatRes.Message.Content, nil
}
