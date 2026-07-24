package embedding

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaProvider_Embed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"embedding": [0.1, 0.2, 0.3]}`))
	}))
	defer ts.Close()

	provider := NewOllamaProvider(ts.URL, "test-model")
	vec, err := provider.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	if len(vec) != 3 {
		t.Fatalf("expected 3 dim vector, got %d", len(vec))
	}
	if vec[0] != 0.1 || vec[1] != 0.2 || vec[2] != 0.3 {
		t.Errorf("unexpected vector values: %v", vec)
	}
}

func TestOllamaProvider_EmbedBatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"embedding": [0.5, 0.5]}`))
	}))
	defer ts.Close()

	provider := NewOllamaProvider(ts.URL, "nomic-embed-text")
	texts := []string{"first document", "second document", "third document"}
	vecs, err := provider.EmbedBatch(context.Background(), texts)
	if err != nil {
		t.Fatalf("EmbedBatch failed: %v", err)
	}

	if len(vecs) != 3 {
		t.Fatalf("expected 3 result vectors, got %d", len(vecs))
	}

	for i, v := range vecs {
		if len(v) != 2 || v[0] != 0.5 {
			t.Errorf("unexpected vector for index %d: %v", i, v)
		}
	}
}

func TestFallbackEmbedIsExplicit(t *testing.T) {
	// Deterministic embeddings are test-only and must be requested explicitly;
	// production providers propagate transport and provider failures.
	vec := fallbackEmbed("fallback test")

	if len(vec) != 384 {
		t.Fatalf("expected 384 dimension fallback vector, got %d", len(vec))
	}
}
