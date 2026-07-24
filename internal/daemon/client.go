package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/KyberixCo/Pharus/internal/config"
	"github.com/KyberixCo/Pharus/internal/i18n"
)

type Client struct {
	cfg        *config.Config
	httpClient *http.Client
}

func NewClient(cfg *config.Config) *Client {
	// Create client with Unix Domain Socket dialer
	udsClient := &http.Client{
		// A synchronous deep run can legitimately exceed ten minutes. The
		// caller context remains the cancellation mechanism for UDS requests.
		Timeout: 0,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return net.DialTimeout("unix", cfg.SocketPath, 3*time.Second)
			},
		},
	}

	return &Client{
		cfg:        cfg,
		httpClient: udsClient,
	}
}

// GetStatus retrieves daemon status.
func (c *Client) GetStatus(ctx context.Context) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/status", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon socket at %s: %w", c.cfg.SocketPath, err)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// SubmitResearch sends a research task to the daemon.
func (c *Client) SubmitResearch(ctx context.Context, topic string) (*ResearchResponse, error) {
	return c.SubmitResearchWithProfile(ctx, topic, "")
}

// SubmitResearchWithProfile sends a research task with an explicit effort
// budget while keeping SubmitResearch backwards compatible.
func (c *Client) SubmitResearchWithProfile(ctx context.Context, topic, profile string) (*ResearchResponse, error) {
	requestedLanguage := c.cfg.Language
	if language, ok := i18n.LanguageFromContext(ctx); ok {
		requestedLanguage = string(language)
	}
	payload, err := json.Marshal(ResearchRequest{Topic: topic, Language: requestedLanguage, Profile: profile})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/research", bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to submit research to daemon: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var res ResearchResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("invalid response from daemon: %s (%w)", string(body), err)
	}

	return &res, nil
}
