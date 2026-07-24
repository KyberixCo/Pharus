//go:build !lancedb_native

package vectordb

import (
	"context"
	"fmt"
)

// LanceDBStore is unavailable unless Pharus is compiled with
// lancedb/lancedb-go and its native library.
type LanceDBStore struct{}

func NewLanceDBStore(string, string, ...EmbeddingFunc) (*LanceDBStore, error) {
	return nil, fmt.Errorf("native LanceDB support is not compiled in; build with -tags lancedb_native and install the lancedb-go native artifacts")
}

func (*LanceDBStore) AddDocuments(context.Context, []Document) error {
	return fmt.Errorf("native LanceDB support is not compiled in")
}
func (*LanceDBStore) SearchSimilar(context.Context, string, int) ([]SearchResult, error) {
	return nil, fmt.Errorf("native LanceDB support is not compiled in")
}
func (*LanceDBStore) SearchSimilarFiltered(context.Context, string, int, map[string]string) ([]SearchResult, error) {
	return nil, fmt.Errorf("native LanceDB support is not compiled in")
}
func (*LanceDBStore) Count() int   { return 0 }
func (*LanceDBStore) Close() error { return nil }

var _ VectorStore = (*LanceDBStore)(nil)
