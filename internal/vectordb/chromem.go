package vectordb

import (
	"context"
	"fmt"
	"os"

	"github.com/philippgille/chromem-go"
)

type Document struct {
	ID        string
	Content   string
	Metadata  map[string]string
	Embedding []float32
}

type SearchResult struct {
	ID         string
	Content    string
	Metadata   map[string]string
	Similarity float32
}

type Store struct {
	db         *chromem.DB
	collection *chromem.Collection
}

// NewStore initializes a chromem-go vector store with disk persistence.
func NewStore(dir string, collectionName string, embedFunc chromem.EmbeddingFunc) (*Store, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create vector db dir %s: %w", dir, err)
	}

	// Persistent DB stored in dir
	db, err := chromem.NewPersistentDB(dir, false)
	if err != nil {
		return nil, fmt.Errorf("failed to init chromem db: %w", err)
	}

	col, err := db.GetOrCreateCollection(collectionName, nil, embedFunc)
	if err != nil {
		return nil, fmt.Errorf("failed to get/create collection %s: %w", collectionName, err)
	}

	return &Store{
		db:         db,
		collection: col,
	}, nil
}

// AddDocuments inserts a slice of documents into the vector collection.
func (s *Store) AddDocuments(ctx context.Context, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}
	chromemDocs := make([]chromem.Document, len(docs))
	for i, d := range docs {
		chromemDocs[i] = chromem.Document{
			ID:        d.ID,
			Content:   d.Content,
			Metadata:  d.Metadata,
			Embedding: d.Embedding,
		}
	}

	return s.collection.AddDocuments(ctx, chromemDocs, 4) // concurrency 4
}

// SearchSimilar queries the collection for top-K relevant documents matching query.
func (s *Store) SearchSimilar(ctx context.Context, query string, nResults int) ([]SearchResult, error) {
	return s.SearchSimilarFiltered(ctx, query, nResults, nil)
}

func (s *Store) SearchSimilarFiltered(ctx context.Context, query string, nResults int, filter map[string]string) ([]SearchResult, error) {
	if nResults <= 0 || s.collection.Count() == 0 {
		return []SearchResult{}, nil
	}
	if nResults > s.collection.Count() {
		nResults = s.collection.Count()
	}
	res, err := s.collection.Query(ctx, query, nResults, filter, nil)
	if err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(res))
	for _, doc := range res {
		results = append(results, SearchResult{
			ID:         doc.ID,
			Content:    doc.Content,
			Metadata:   doc.Metadata,
			Similarity: doc.Similarity,
		})
	}

	return results, nil
}

func (s *Store) Count() int { return s.collection.Count() }

// Close flushes or releases resources for the chromem store.
func (s *Store) Close() error {
	return nil
}

var _ VectorStore = (*Store)(nil)
