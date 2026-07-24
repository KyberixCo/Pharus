package research

import (
	"context"
	"testing"

	"github.com/KyberixCo/Pharus/internal/vectordb"
	"github.com/philippgille/chromem-go"
)

func TestSearchEvidenceQueriesConfiguredStore(t *testing.T) {
	embed := func(_ context.Context, text string) ([]float32, error) {
		if text == "relevant query" || text == "relevant evidence" {
			return []float32{1, 0}, nil
		}
		return []float32{0, 1}, nil
	}
	store, err := vectordb.NewStore(t.TempDir(), "mcp_search", chromem.EmbeddingFunc(embed))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddDocuments(context.Background(), []vectordb.Document{
		{ID: "real-1", Content: "relevant evidence", Metadata: map[string]string{"research_id": "r1"}},
		{ID: "other", Content: "unrelated", Metadata: map[string]string{"research_id": "r2"}},
	}); err != nil {
		t.Fatal(err)
	}
	engine := &Engine{vectorDB: store}
	results, err := engine.SearchEvidenceFiltered(context.Background(), "relevant query", 5, map[string]string{"research_id": "r1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != "real-1" {
		t.Fatalf("expected real stored evidence, got %+v", results)
	}
}
