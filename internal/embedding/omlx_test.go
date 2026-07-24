package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOMLXProviderEmbed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var request mlxEmbedReq
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Model != "mlx-community/test-model" {
			t.Fatalf("unexpected model ID: %s", request.Model)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer server.Close()

	vec, err := NewOMLXProvider(server.URL+"/", "mlx-community/test-model").Embed(context.Background(), "test")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	if len(vec) != 3 {
		t.Fatalf("expected 3 dimensions, got %d", len(vec))
	}
}

func TestOMLXProviderSerializesRequests(t *testing.T) {
	var active int32
	var maxActive int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&active, 1)
		defer atomic.AddInt32(&active, -1)
		for {
			observed := atomic.LoadInt32(&maxActive)
			if current <= observed || atomic.CompareAndSwapInt32(&maxActive, observed, current) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1]}]}`))
	}))
	defer server.Close()

	provider := NewOMLXProvider(server.URL, "test-model")
	var group sync.WaitGroup
	for range 4 {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := provider.Embed(context.Background(), "test"); err != nil {
				t.Errorf("Embed failed: %v", err)
			}
		}()
	}
	group.Wait()

	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Fatalf("expected one in-flight OMLX request, got %d", got)
	}
}

func TestOMLXProviderChunksAndCombinesLongInput(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request mlxEmbedReq
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got := len([]rune(request.Input)); got > omlxEmbeddingChunkRunes {
			t.Fatalf("chunk has %d runes; maximum is %d", got, omlxEmbeddingChunkRunes)
		}
		atomic.AddInt32(&requests, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[3,4]}]}`))
	}))
	defer server.Close()

	input := strings.Repeat("a", omlxEmbeddingChunkRunes+1)
	vec, err := NewOMLXProvider(server.URL, "test-model").Embed(context.Background(), input)
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("expected 2 chunk requests, got %d", got)
	}
	if len(vec) != 2 || vec[0] < 0.599 || vec[0] > 0.601 || vec[1] < 0.799 || vec[1] > 0.801 {
		t.Fatalf("unexpected normalized combined vector: %v", vec)
	}
}

func TestOMLXProviderIncludesAPIErrorMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"Model 'missing' not found. Available models: installed"}}`))
	}))
	defer server.Close()

	_, err := NewOMLXProvider(server.URL, "missing").Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"HTTP status 404", "Model 'missing' not found", "Available models: installed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err, want)
		}
	}
}
