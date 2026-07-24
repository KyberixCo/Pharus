package anthropic

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
			Timeout: 120 * time.Second,
		},
	}
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messagesRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Temperature float64            `json:"temperature"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type messagesResponse struct {
	Content []contentBlock `json:"content"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *Client) GenerateCompletion(ctx context.Context, messages []types.Message, temperature float64) (string, error) {
	if c.cfg == nil || c.cfg.Anthropic.APIKey == "" {
		return "", fmt.Errorf("Anthropic API key is missing. Set PHARUS_ANTHROPIC_API_KEY env or configure in config.yaml")
	}

	modelName := c.cfg.Anthropic.Model
	if modelName == "" {
		modelName = "claude-3-5-sonnet-20241022"
	}

	baseURL := c.cfg.Anthropic.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}

	var systemPrompt string
	var anthropicMsgs []anthropicMessage

	for _, msg := range messages {
		if msg.Role == "system" {
			if systemPrompt != "" {
				systemPrompt += "\n" + msg.Content
			} else {
				systemPrompt = msg.Content
			}
		} else {
			role := msg.Role
			if role != "user" && role != "assistant" {
				role = "user"
			}
			anthropicMsgs = append(anthropicMsgs, anthropicMessage{
				Role:    role,
				Content: msg.Content,
			})
		}
	}

	// Anthropic exige al menos un mensaje de usuario si solo se proveyó sistema
	if len(anthropicMsgs) == 0 && systemPrompt != "" {
		anthropicMsgs = append(anthropicMsgs, anthropicMessage{
			Role:    "user",
			Content: systemPrompt,
		})
		systemPrompt = ""
	}

	reqPayload := messagesRequest{
		Model:       modelName,
		MaxTokens:   4096,
		System:      systemPrompt,
		Messages:    anthropicMsgs,
		Temperature: temperature,
	}

	data, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal anthropic request: %w", err)
	}

	url := strings.TrimRight(baseURL, "/") + "/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(data))
	if err != nil {
		return "", fmt.Errorf("failed to create anthropic http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.cfg.Anthropic.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	log := logger.Get()
	log.Debug("Sending request to Anthropic LLM", "url", url, "model", modelName)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Anthropic HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read Anthropic response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Anthropic API error (status %d): %s", resp.StatusCode, string(body))
	}

	var msgRes messagesResponse
	if err := json.Unmarshal(body, &msgRes); err != nil {
		return "", fmt.Errorf("failed to parse Anthropic JSON response: %s (%w)", string(body), err)
	}

	if msgRes.Error != nil && msgRes.Error.Message != "" {
		return "", fmt.Errorf("Anthropic API internal error: %s", msgRes.Error.Message)
	}

	var sb strings.Builder
	for _, block := range msgRes.Content {
		if block.Type == "text" || block.Text != "" {
			sb.WriteString(block.Text)
		}
	}

	if sb.Len() == 0 {
		return "", fmt.Errorf("Anthropic returned empty content in response")
	}

	return sb.String(), nil
}
