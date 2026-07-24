package scraper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
)

const (
	defaultSearXNGTimeout         = 10 * time.Second
	defaultSearXNGMaxResponseSize = 2 << 20 // 2 MiB
	searXNGUserAgent              = "Pharus/1.0 (+https://github.com/KyberixCo/Pharus)"
)

// SearXNGHTTPError preserves the server status while keeping the response body
// bounded and safe to display in diagnostics.
type SearXNGHTTPError struct {
	StatusCode int
	Body       string
}

func (e *SearXNGHTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("SearXNG returned HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("SearXNG returned HTTP %d: %s", e.StatusCode, e.Body)
}

type SearXNGResponseTooLargeError struct{ Limit int64 }

func (e *SearXNGResponseTooLargeError) Error() string {
	return fmt.Sprintf("SearXNG response exceeds configured limit of %d bytes", e.Limit)
}

type SearXNGInvalidResponseError struct{ Reason string }

func (e *SearXNGInvalidResponseError) Error() string {
	return "invalid SearXNG JSON response: " + e.Reason
}

type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

type SearXNGClient struct {
	baseURL         *url.URL
	client          *http.Client
	maxResponseSize int64
}

type searxngResponse struct {
	Results json.RawMessage `json:"results"`
}

// NewSearXNGClient constructs a client with safe production defaults.
func NewSearXNGClient(baseURL string) *SearXNGClient {
	client, err := NewSearXNGClientWithOptions(baseURL, defaultSearXNGTimeout, defaultSearXNGMaxResponseSize)
	if err == nil {
		return client
	}
	// Keep the existing constructor API. Search will return this parse error when
	// called, instead of panicking during engine construction.
	return &SearXNGClient{client: &http.Client{Timeout: defaultSearXNGTimeout}, maxResponseSize: defaultSearXNGMaxResponseSize}
}

// NewSearXNGClientWithOptions allows deployments to tune timeout and response
// size while retaining the same request validation.
func NewSearXNGClientWithOptions(baseURL string, timeout time.Duration, maxResponseSize int64) (*SearXNGClient, error) {
	if baseURL == "" {
		baseURL = "http://localhost:8090"
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil {
		if err == nil {
			err = errors.New("must be an absolute HTTP(S) URL without credentials")
		}
		return nil, fmt.Errorf("invalid SearXNG base URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("invalid SearXNG base URL: unsupported scheme %q", u.Scheme)
	}
	if timeout <= 0 {
		timeout = defaultSearXNGTimeout
	}
	if maxResponseSize <= 0 {
		maxResponseSize = defaultSearXNGMaxResponseSize
	}
	return &SearXNGClient{baseURL: u, client: &http.Client{Timeout: timeout}, maxResponseSize: maxResponseSize}, nil
}

func (s *SearXNGClient) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	return s.SearchWithCategories(ctx, query, nil, maxResults)
}

func (s *SearXNGClient) SearchWithCategories(ctx context.Context, query string, categories []string, maxResults int) ([]SearchResult, error) {
	if s.baseURL == nil {
		return nil, errors.New("invalid SearXNG base URL")
	}
	u := *s.baseURL
	u.Path = path.Join(u.Path, "search")
	q := u.Query()
	q.Set("q", query)
	q.Set("format", "json")
	if len(categories) > 0 {
		q.Set("categories", strings.Join(categories, ","))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create SearXNG request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", searXNGUserAgent)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SearXNG request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &SearXNGHTTPError{StatusCode: resp.StatusCode, Body: readSanitizedBody(resp.Body, 4096)}
	}
	if !isJSONContentType(resp.Header.Get("Content-Type")) {
		return nil, &SearXNGInvalidResponseError{Reason: fmt.Sprintf("expected JSON Content-Type, got %q", resp.Header.Get("Content-Type"))}
	}

	body, err := readLimited(resp.Body, s.maxResponseSize)
	if err != nil {
		return nil, err
	}
	var payload searxngResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, &SearXNGInvalidResponseError{Reason: err.Error()}
	}
	if len(payload.Results) == 0 || string(payload.Results) == "null" {
		return nil, &SearXNGInvalidResponseError{Reason: "missing results array"}
	}
	var rawResults []SearchResult
	if err := json.Unmarshal(payload.Results, &rawResults); err != nil {
		return nil, &SearXNGInvalidResponseError{Reason: "results must be an array"}
	}

	seenURLs := make(map[string]bool)
	results := make([]SearchResult, 0, len(rawResults))
	for _, result := range rawResults {
		if maxResults > 0 && len(results) >= maxResults {
			break
		}
		if result.URL == "" || seenURLs[result.URL] {
			continue
		}
		seenURLs[result.URL] = true
		results = append(results, result)
	}
	return results, nil
}

// CheckJSONCapability performs the representative operation required by Pharus.
func (s *SearXNGClient) CheckJSONCapability(ctx context.Context) error {
	_, err := s.Search(ctx, "pharus healthcheck", 1)
	return err
}

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read SearXNG response: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, &SearXNGResponseTooLargeError{Limit: limit}
	}
	return body, nil
}

var sensitiveValue = regexp.MustCompile(`(?i)(api[_-]?key|authorization|password|secret|token)\s*[:=]\s*[^\s,;]+`)

func readSanitizedBody(r io.Reader, limit int64) string {
	body, _ := io.ReadAll(io.LimitReader(r, limit))
	text := strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, string(body)))
	return sensitiveValue.ReplaceAllString(text, "$1=[redacted]")
}

func isJSONContentType(contentType string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}
