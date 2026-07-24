package scraper

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDistillHTMLAndChunker(t *testing.T) {
	rawHTML := []byte(`
		<!DOCTYPE html>
		<html>
		<head><title>Pharus Test Page</title></head>
		<body>
			<script>console.log("ignore me");</script>
			<h1>Deep Research Architecture</h1>
			<p>Pharus is a local deep research engine built in Go using the Model Context Protocol.</p>
			<p>It provides neural search capabilities, embedded vector databases, and secure scraping.</p>
		</body>
		</html>
	`)

	distilled, err := DistillHTML(rawHTML, "https://pharus.ai/docs")
	if err != nil {
		t.Fatalf("DistillHTML failed: %v", err)
	}

	if distilled.Title != "Pharus Test Page" {
		t.Errorf("expected title 'Pharus Test Page', got '%s'", distilled.Title)
	}

	if len(distilled.TextContent) == 0 {
		t.Fatal("expected non-empty distilled text content")
	}

	// Test chunker
	chunks := ChunkDocument(distilled, 5, 2)
	if len(chunks) == 0 {
		t.Fatal("expected at least 1 document chunk")
	}

	if chunks[0].SourceURL != "https://pharus.ai/docs" {
		t.Errorf("expected source URL 'https://pharus.ai/docs', got '%s'", chunks[0].SourceURL)
	}
}

func TestSSRFProtection(t *testing.T) {
	// Test isPrivateOrLoopbackIP logic directly
	loopback := net.ParseIP("127.0.0.1")
	if !isPrivateOrLoopbackIP(loopback) {
		t.Error("expected 127.0.0.1 to be flagged as loopback/private IP")
	}

	privateIP := net.ParseIP("192.168.1.10")
	if !isPrivateOrLoopbackIP(privateIP) {
		t.Error("expected 192.168.1.10 to be flagged as private IP")
	}

	ipv4Mapped := net.ParseIP("::ffff:127.0.0.1")
	if !isPrivateOrLoopbackIP(ipv4Mapped) {
		t.Error("expected ::ffff:127.0.0.1 to be flagged as private IP")
	}

	publicIP := net.ParseIP("8.8.8.8")
	if isPrivateOrLoopbackIP(publicIP) {
		t.Error("expected 8.8.8.8 NOT to be flagged as private IP")
	}
}

func TestSearXNGClient_Search(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"results": [
				{"title": "Result 1", "url": "https://example.com/1", "content": "Content 1"},
				{"title": "Result 1 Duplicate", "url": "https://example.com/1", "content": "Dup Content"},
				{"title": "Result 2", "url": "https://example.com/2", "content": "Content 2"}
			]
		}`))
	}))
	defer ts.Close()

	client := NewSearXNGClient(ts.URL)
	results, err := client.SearchWithCategories(context.Background(), "go programming", []string{"general"}, 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Should deduplicate duplicate URL
	if len(results) != 2 {
		t.Fatalf("expected 2 deduplicated results, got %d", len(results))
	}

	if results[0].Title != "Result 1" || results[1].Title != "Result 2" {
		t.Errorf("unexpected results: %+v", results)
	}
}

func TestSearXNGClient_SearchHTTPErrorPreservesStatusAndSanitizesBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("token=do-not-log forbidden"))
	}))
	defer ts.Close()

	_, err := NewSearXNGClient(ts.URL).Search(context.Background(), "health", 1)
	var httpErr *SearXNGHTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected SearXNGHTTPError, got %v", err)
	}
	if httpErr.StatusCode != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", httpErr.StatusCode)
	}
	if strings.Contains(httpErr.Body, "do-not-log") {
		t.Fatalf("error body leaked sensitive data: %q", httpErr.Body)
	}
}

func TestSearXNGClient_CheckJSONCapabilityRejectsHTML(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "pharus healthcheck" || r.URL.Query().Get("format") != "json" {
			t.Fatalf("unexpected healthcheck request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>format disabled</html>"))
	}))
	defer ts.Close()

	err := NewSearXNGClient(ts.URL).CheckJSONCapability(context.Background())
	var invalid *SearXNGInvalidResponseError
	if !errors.As(err, &invalid) {
		t.Fatalf("expected invalid response error, got %v", err)
	}
}

func TestSearXNGClient_RejectsOversizedResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"` + strings.Repeat("x", 128) + `","url":"https://example.com"}]}`))
	}))
	defer ts.Close()

	client, err := NewSearXNGClientWithOptions(ts.URL, time.Second, 32)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Search(context.Background(), "health", 1)
	var oversized *SearXNGResponseTooLargeError
	if !errors.As(err, &oversized) {
		t.Fatalf("expected response-too-large error, got %v", err)
	}
}
