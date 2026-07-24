package minimax

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/KyberixCo/Pharus/internal/config"
	"github.com/KyberixCo/Pharus/internal/llm/types"
	"github.com/KyberixCo/Pharus/pkg/logger"
)

const (
	defaultBaseURL   = "https://api.minimax.io/anthropic"
	defaultMaxTokens = 16384
	maxMaxTokens     = 204800
)

type Client struct {
	cfg        *config.Config
	httpClient *http.Client
}

func NewClient(cfg *config.Config) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

// GenerateCompletion calls MiniMax through its recommended Anthropic-compatible
// Messages API. It rejects length-limited responses instead of returning a
// partial report as a successful completion.
func (c *Client) GenerateCompletion(ctx context.Context, messages []types.Message, temperature float64) (string, error) {
	if c.cfg == nil || c.cfg.MiniMax.APIKey == "" {
		return "", fmt.Errorf("MiniMax API key is missing. Configure in ~/.pharus/config.yaml or set PHARUS_MINIMAX_API_KEY")
	}

	modelName := c.cfg.MiniMax.Model
	if modelName == "" || strings.EqualFold(modelName, "minimax-m3") {
		modelName = "MiniMax-M3"
	}

	maxTokens := c.cfg.MiniMax.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	if maxTokens > maxMaxTokens {
		return "", fmt.Errorf("MiniMax max_tokens %d exceeds the supported maximum of %d", maxTokens, maxMaxTokens)
	}
	requestOptions := types.RequestOptionsFromContext(ctx)
	if requestOptions.MaxTokens > 0 && requestOptions.MaxTokens < maxTokens {
		maxTokens = requestOptions.MaxTokens
	}

	var systemPrompt string
	inputMessages := make([]AnthropicMessage, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "system" || msg.Role == "developer" {
			if systemPrompt != "" {
				systemPrompt += "\n" + msg.Content
			} else {
				systemPrompt = msg.Content
			}
			continue
		}

		role := msg.Role
		if role != "assistant" {
			role = "user"
		}
		inputMessages = append(inputMessages, AnthropicMessage{
			Role: role,
			Content: []ContentBlock{{
				Type: "text",
				Text: msg.Content,
			}},
		})
	}
	if len(inputMessages) == 0 && systemPrompt != "" {
		inputMessages = append(inputMessages, AnthropicMessage{
			Role: "user",
			Content: []ContentBlock{{
				Type: "text",
				Text: systemPrompt,
			}},
		})
		systemPrompt = ""
	}

	if temperature <= 0 {
		temperature = 0.2
	} else if temperature > 1 {
		temperature = 1
	}

	payload := MessagesRequest{
		Model:       modelName,
		MaxTokens:   maxTokens,
		System:      systemPrompt,
		Messages:    inputMessages,
		Temperature: temperature,
		TopP:        0.95,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal MiniMax Anthropic request: %w", err)
	}

	endpoint, err := resolveEndpointURL(c.cfg.MiniMax.BaseURL)
	if err != nil {
		return "", fmt.Errorf("invalid MiniMax base URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to create MiniMax HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", c.cfg.MiniMax.APIKey)
	req.Header.Set("Anthropic-Version", "2023-06-01")

	log := logger.Get()
	log.Debug("Sending request to MiniMax Anthropic-compatible API", "url", endpoint, "model", modelName, "max_tokens", maxTokens)

	requestCtx := ctx
	cancel := func() {}
	if requestOptions.Timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, requestOptions.Timeout)
		req = req.WithContext(requestCtx)
	}
	defer cancel()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("MiniMax HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read MiniMax response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("MiniMax API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result MessagesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse MiniMax JSON response: %s (%w)", string(body), err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("MiniMax API execution failed: %s", result.Error.Message)
	}
	if result.BaseResp.StatusCode != 0 {
		return "", fmt.Errorf("MiniMax API execution failed (code %d): %s", result.BaseResp.StatusCode, result.BaseResp.StatusMsg)
	}
	if result.StopReason == "max_tokens" {
		return "", fmt.Errorf("MiniMax generation truncated after reaching max_tokens=%d", maxTokens)
	}
	if result.StopReason != "" && result.StopReason != "end_turn" && result.StopReason != "stop_sequence" {
		return "", fmt.Errorf("MiniMax generation stopped unexpectedly: %s", result.StopReason)
	}

	var output strings.Builder
	for _, block := range result.Content {
		if block.Type == "text" {
			output.WriteString(block.Text)
		}
	}
	if output.Len() == 0 {
		return "", fmt.Errorf("MiniMax returned empty text content")
	}
	return output.String(), nil
}

func resolveEndpointURL(rawBaseURL string) (string, error) {
	baseURL := strings.TrimSpace(rawBaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported URL scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("URL has no host")
	}

	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case strings.HasSuffix(path, "/messages"):
		parsed.Path = path
	case strings.HasSuffix(path, "/anthropic/v1"):
		parsed.Path = path + "/messages"
	case strings.HasSuffix(path, "/anthropic"):
		parsed.Path = path + "/v1/messages"
	case parsed.Hostname() == "api.minimax.io" && (path == "" || path == "/v1" || path == "/v1/responses"):
		parsed.Path = "/anthropic/v1/messages"
	case strings.HasSuffix(path, "/v1"):
		parsed.Path = path + "/messages"
	default:
		parsed.Path = path + "/v1/messages"
	}
	return parsed.String(), nil
}
