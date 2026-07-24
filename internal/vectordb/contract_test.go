package vectordb

import (
	"context"
	"fmt"
	"testing"

	"github.com/philippgille/chromem-go"
)

func TestVectorStoreContract(t *testing.T) {
	builders := map[string]func(t *testing.T) VectorStore{
		"chromem": func(t *testing.T) VectorStore {
			t.Helper()
			store, err := NewStore(t.TempDir(), "contract", chromem.EmbeddingFunc(contractEmbedding))
			if err != nil {
				t.Fatal(err)
			}
			return store
		},
	}

	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			store := build(t)
			t.Cleanup(func() { _ = store.Close() })
			assertSearchCount(t, store, ctx, 0, 0)
			if err := store.AddDocuments(ctx, nil); err != nil {
				t.Fatalf("empty add: %v", err)
			}
			for _, count := range []int{1, 3, 10} {
				docs := make([]Document, count)
				for i := range docs {
					docs[i] = Document{ID: fmt.Sprintf("%d-%d", count, i), Content: fmt.Sprintf("topic %d", i), Metadata: map[string]string{"research_id": "current"}}
				}
				if err := store.AddDocuments(ctx, docs); err != nil {
					t.Fatalf("add %d: %v", count, err)
				}
				for _, topK := range []int{0, 1, 7} {
					assertSearchCount(t, store, ctx, topK, topK)
				}
			}
			results, err := store.SearchSimilarFiltered(ctx, "topic", 7, map[string]string{"research_id": "other"})
			if err != nil || len(results) != 0 {
				t.Fatalf("scope leak: results=%d err=%v", len(results), err)
			}
		})
	}
}

func assertSearchCount(t *testing.T, store VectorStore, ctx context.Context, topK, wantMax int) {
	t.Helper()
	results, err := store.SearchSimilarFiltered(ctx, "topic", topK, map[string]string{"research_id": "current"})
	if err != nil {
		t.Fatalf("search topK=%d: %v", topK, err)
	}
	if len(results) > wantMax {
		t.Fatalf("topK=%d returned %d", topK, len(results))
	}
	if topK == 0 && len(results) != 0 {
		t.Fatalf("zero topK returned %d", len(results))
	}
}

func contractEmbedding(_ context.Context, text string) ([]float32, error) {
	if text == "fail" {
		return nil, fmt.Errorf("embedding unavailable")
	}
	return []float32{1, 0, float32(len(text) % 2)}, nil
}
