//go:build lancedb_native

package vectordb

import (
	"context"
	"os"
	"testing"
)

func TestLanceDBStore_AddAndSearch(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "pharus_lancedb_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mockEmbedFunc := func(ctx context.Context, text string) ([]float32, error) {
		if text == "deep learning research" || text == "deep learning" {
			return []float32{1.0, 0.0, 0.0}, nil
		}
		if text == "quantum computing mechanics" {
			return []float32{0.0, 1.0, 0.0}, nil
		}
		return []float32{0.5, 0.5, 0.0}, nil
	}

	store, err := NewLanceDBStore(tempDir, "test_collection", mockEmbedFunc)
	if err != nil {
		t.Fatalf("failed to init LanceDBStore: %v", err)
	}

	docs := []Document{
		{
			ID:        "doc1",
			Content:   "deep learning research",
			Metadata:  map[string]string{"category": "AI"},
			Embedding: []float32{1.0, 0.0, 0.0},
		},
		{
			ID:        "doc2",
			Content:   "quantum computing mechanics",
			Metadata:  map[string]string{"category": "Physics"},
			Embedding: []float32{0.0, 1.0, 0.0},
		},
	}

	ctx := context.Background()
	if err := store.AddDocuments(ctx, docs); err != nil {
		t.Fatalf("failed to add documents: %v", err)
	}

	// Search for AI topic
	results, err := store.SearchSimilar(ctx, "deep learning", 1)
	if err != nil {
		t.Fatalf("failed to search: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "doc1" {
		t.Errorf("expected doc1 as top result, got %s", results[0].ID)
	}

	// Close store
	if err := store.Close(); err != nil {
		t.Fatalf("failed to close store: %v", err)
	}

	// Verify persistence by re-opening store
	reopenedStore, err := NewLanceDBStore(tempDir, "test_collection", mockEmbedFunc)
	if err != nil {
		t.Fatalf("failed to re-open LanceDBStore: %v", err)
	}

	reopenedResults, err := reopenedStore.SearchSimilar(ctx, "deep learning", 2)
	if err != nil {
		t.Fatalf("failed to search in reopened store: %v", err)
	}

	if len(reopenedResults) < 1 || reopenedResults[0].ID != "doc1" {
		t.Errorf("expected persisted doc1, got %v", reopenedResults)
	}
}

func TestCosineSimilarity(t *testing.T) {
	v1 := []float32{1.0, 0.0, 0.0}
	v2 := []float32{1.0, 0.0, 0.0}
	sim := CosineSimilarity(v1, v2)
	if sim < 0.99 {
		t.Errorf("expected ~1.0 cosine similarity for identical vectors, got %f", sim)
	}

	v3 := []float32{0.0, 1.0, 0.0}
	simOrthogonal := CosineSimilarity(v1, v3)
	if simOrthogonal > 0.01 {
		t.Errorf("expected 0.0 cosine similarity for orthogonal vectors, got %f", simOrthogonal)
	}
}

func TestLanceDBEmbeddingErrorsPropagate(t *testing.T) {
	store, err := NewLanceDBStore(t.TempDir(), "errors", contractEmbedding)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddDocuments(context.Background(), []Document{{ID: "bad", Content: "fail"}}); err == nil {
		t.Fatal("expected add embedding error")
	}
}

func TestLanceDBStoreUpsertsPersistedDocument(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLanceDBStore(dir, "upsert", contractEmbedding)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	doc := Document{ID: "stable", Content: "topic", Metadata: map[string]string{"research_id": "current"}}
	if err := store.AddDocuments(ctx, []Document{doc}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewLanceDBStore(dir, "upsert", contractEmbedding)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.AddDocuments(ctx, []Document{doc}); err != nil {
		t.Fatal(err)
	}
	if reopened.Count() != 1 {
		t.Fatalf("expected upsert after reopen to retain one document, got %d", reopened.Count())
	}
}
