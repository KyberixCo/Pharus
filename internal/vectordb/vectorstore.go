package vectordb

import (
	"context"
	"math"
)

// EmbeddingFunc converts text into a vector in the collection's embedding
// space.
type EmbeddingFunc func(ctx context.Context, text string) ([]float32, error)

// CosineSimilarity returns zero for empty or mismatched vectors.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}

// VectorStore defines an agnostic interface for embedded vector storage engines.
type VectorStore interface {
	AddDocuments(ctx context.Context, docs []Document) error
	SearchSimilar(ctx context.Context, query string, nResults int) ([]SearchResult, error)
	// SearchSimilarFiltered restricts results to exact metadata values. It is
	// used to prevent one research execution from reading another's corpus.
	SearchSimilarFiltered(ctx context.Context, query string, nResults int, filter map[string]string) ([]SearchResult, error)
	Count() int
	Close() error
}
