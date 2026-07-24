//go:build lancedb_native

package vectordb

import (
	"context"
	"testing"
)

func TestLanceDBStore(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewLanceDBStore(tmpDir, "test_collection", func(_ context.Context, text string) ([]float32, error) {
		if text == "Architecture" || text == "Pharus Deep Research Engine Architecture" {
			return []float32{1, 0}, nil
		}
		return []float32{0, 1}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error creating LanceDB store: %v", err)
	}

	docs := []Document{
		{ID: "1", Content: "Pharus Deep Research Engine Architecture", Metadata: map[string]string{"type": "doc"}},
		{ID: "2", Content: "DataSTORM and Co-STORM Orquestation", Metadata: map[string]string{"type": "doc"}},
	}

	ctx := context.Background()
	if err := store.AddDocuments(ctx, docs); err != nil {
		t.Fatalf("unexpected error adding docs: %v", err)
	}

	results, err := store.SearchSimilar(ctx, "Architecture", 1)
	if err != nil {
		t.Fatalf("unexpected error searching docs: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].ID != "1" {
		t.Errorf("expected doc ID 1, got %s", results[0].ID)
	}

	if err := store.Close(); err != nil {
		t.Errorf("unexpected error closing store: %v", err)
	}
}
