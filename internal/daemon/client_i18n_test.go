package daemon

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/KyberixCo/Pharus/internal/config"
	"github.com/KyberixCo/Pharus/internal/i18n"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestSubmitResearchPropagatesContextLanguage(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Language = "auto"
	client := NewClient(cfg)
	var captured ResearchRequest
	client.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(req.Body).Decode(&captured); err != nil {
			t.Fatal(err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"status":"success","elapsed":"0s"}`)),
			Request:    req,
		}, nil
	})}

	ctx := i18n.WithLanguage(context.Background(), i18n.Spanish)
	if _, err := client.SubmitResearch(ctx, "topic"); err != nil {
		t.Fatal(err)
	}
	if captured.Language != "es" {
		t.Fatalf("expected Spanish to be sent to daemon, got %q", captured.Language)
	}
}
