//go:build lancedb_native

package vectordb

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
	"github.com/apache/arrow/go/v17/arrow/memory"
	"github.com/lancedb/lancedb-go/pkg/contracts"
	native "github.com/lancedb/lancedb-go/pkg/lancedb"
)

type LanceDBStore struct {
	mu         sync.RWMutex
	connection contracts.IConnection
	table      contracts.ITable
	tableName  string
	embedFunc  EmbeddingFunc
	dimension  int
}

// NewLanceDBStore opens a real LanceDB database through lancedb/lancedb-go.
// Building this implementation requires the lancedb_native build tag and the
// native library documented by github.com/lancedb/lancedb-go.
func NewLanceDBStore(dir, collectionName string, embedFunc ...EmbeddingFunc) (*LanceDBStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create LanceDB directory: %w", err)
	}
	connection, err := native.Connect(context.Background(), dir, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to native LanceDB: %w", err)
	}
	store := &LanceDBStore{connection: connection, tableName: collectionName}
	if len(embedFunc) > 0 {
		store.embedFunc = embedFunc[0]
	}
	names, err := connection.TableNames(context.Background())
	if err != nil {
		connection.Close()
		return nil, fmt.Errorf("list LanceDB tables: %w", err)
	}
	for _, name := range names {
		if name != collectionName {
			continue
		}
		store.table, err = connection.OpenTable(context.Background(), collectionName)
		if err != nil {
			connection.Close()
			return nil, fmt.Errorf("open LanceDB table %q: %w", collectionName, err)
		}
		schema, schemaErr := store.table.Schema(context.Background())
		if schemaErr != nil {
			store.Close()
			return nil, fmt.Errorf("read LanceDB schema: %w", schemaErr)
		}
		for _, field := range schema.Fields() {
			if field.Name != "vector" {
				continue
			}
			if list, listOK := field.Type.(*arrow.FixedSizeListType); listOK {
				store.dimension = int(list.Len())
			}
			break
		}
		break
	}
	return store, nil
}

func (s *LanceDBStore) AddDocuments(ctx context.Context, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}
	prepared := make([]Document, len(docs))
	copy(prepared, docs)
	for i := range prepared {
		if len(prepared[i].Embedding) == 0 && s.embedFunc != nil {
			vector, err := s.embedFunc(ctx, prepared[i].Content)
			if err != nil {
				return fmt.Errorf("embed document %q: %w", prepared[i].ID, err)
			}
			prepared[i].Embedding = vector
		}
		if len(prepared[i].Embedding) == 0 {
			return fmt.Errorf("document %q has no embedding", prepared[i].ID)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dimension == 0 {
		s.dimension = len(prepared[0].Embedding)
		if err := s.createTable(ctx); err != nil {
			return err
		}
	}
	for _, doc := range prepared {
		if len(doc.Embedding) != s.dimension {
			return fmt.Errorf("embedding dimension mismatch: table has %d, document has %d", s.dimension, len(doc.Embedding))
		}
	}

	for _, doc := range prepared {
		if err := s.table.Delete(ctx, "id = '"+escapeSQLLiteral(doc.ID)+"'"); err != nil {
			return fmt.Errorf("delete existing LanceDB document %q: %w", doc.ID, err)
		}
	}
	record, err := documentsRecord(prepared, s.dimension)
	if err != nil {
		return err
	}
	defer record.Release()
	if err := s.table.Add(ctx, record, &contracts.AddDataOptions{Mode: contracts.WriteModeAppend}); err != nil {
		return fmt.Errorf("add documents to LanceDB: %w", err)
	}
	return nil
}

func (s *LanceDBStore) createTable(ctx context.Context) error {
	fields := []arrow.Field{
		{Name: "id", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "content", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "metadata", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "vector", Type: arrow.FixedSizeListOf(int32(s.dimension), arrow.PrimitiveTypes.Float32), Nullable: false},
	}
	schema, err := native.NewSchema(arrow.NewSchema(fields, nil))
	if err != nil {
		return fmt.Errorf("create LanceDB schema: %w", err)
	}
	s.table, err = s.connection.CreateTable(ctx, s.tableName, schema)
	if err != nil {
		return fmt.Errorf("create LanceDB table %q: %w", s.tableName, err)
	}
	return nil
}

func documentsRecord(docs []Document, dimension int) (arrow.Record, error) {
	pool := memory.NewGoAllocator()
	idBuilder := array.NewStringBuilder(pool)
	defer idBuilder.Release()
	contentBuilder := array.NewStringBuilder(pool)
	defer contentBuilder.Release()
	metadataBuilder := array.NewStringBuilder(pool)
	defer metadataBuilder.Release()
	vectorBuilder := array.NewFixedSizeListBuilder(pool, int32(dimension), arrow.PrimitiveTypes.Float32)
	defer vectorBuilder.Release()
	values := vectorBuilder.ValueBuilder().(*array.Float32Builder)

	for _, doc := range docs {
		metadata, err := json.Marshal(doc.Metadata)
		if err != nil {
			return nil, fmt.Errorf("serialize metadata for %q: %w", doc.ID, err)
		}
		idBuilder.Append(doc.ID)
		contentBuilder.Append(doc.Content)
		metadataBuilder.Append(string(metadata))
		vectorBuilder.Append(true)
		values.AppendValues(doc.Embedding, nil)
	}
	columns := []arrow.Array{
		idBuilder.NewArray(), contentBuilder.NewArray(), metadataBuilder.NewArray(), vectorBuilder.NewArray(),
	}
	for _, column := range columns {
		defer column.Release()
	}
	schema := arrow.NewSchema([]arrow.Field{
		{Name: "id", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "content", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "metadata", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "vector", Type: arrow.FixedSizeListOf(int32(dimension), arrow.PrimitiveTypes.Float32), Nullable: false},
	}, nil)
	return array.NewRecord(schema, columns, int64(len(docs))), nil
}

func (s *LanceDBStore) SearchSimilar(ctx context.Context, query string, nResults int) ([]SearchResult, error) {
	return s.SearchSimilarFiltered(ctx, query, nResults, nil)
}

func (s *LanceDBStore) SearchSimilarFiltered(ctx context.Context, query string, nResults int, filter map[string]string) ([]SearchResult, error) {
	if nResults <= 0 {
		return []SearchResult{}, nil
	}
	if s.embedFunc == nil {
		return nil, fmt.Errorf("LanceDB search requires an embedding function")
	}
	vector, err := s.embedFunc(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.table == nil {
		return []SearchResult{}, nil
	}
	searchLimit := nResults
	if len(filter) > 0 {
		count, countErr := s.table.Count(ctx)
		if countErr != nil {
			return nil, fmt.Errorf("count LanceDB rows for filtered search: %w", countErr)
		}
		searchLimit = int(count)
	}
	if searchLimit == 0 {
		return []SearchResult{}, nil
	}
	rows, err := s.table.VectorSearch(ctx, "vector", vector, searchLimit)
	if err != nil {
		return nil, fmt.Errorf("native LanceDB vector search: %w", err)
	}
	results := make([]SearchResult, 0, nResults)
	for _, row := range rows {
		var metadata map[string]string
		_ = json.Unmarshal([]byte(stringValue(row["metadata"])), &metadata)
		if !matchesMetadata(metadata, filter) {
			continue
		}
		distance := floatValue(row["_distance"])
		results = append(results, SearchResult{
			ID: stringValue(row["id"]), Content: stringValue(row["content"]), Metadata: metadata,
			Similarity: float32(1 / (1 + math.Max(distance, 0))),
		})
		if len(results) == nResults {
			break
		}
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Similarity > results[j].Similarity })
	return results, nil
}

func (s *LanceDBStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.table == nil {
		return 0
	}
	count, err := s.table.Count(context.Background())
	if err != nil {
		return 0
	}
	return int(count)
}

func (s *LanceDBStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var errs []string
	if s.table != nil {
		if err := s.table.Close(); err != nil {
			errs = append(errs, err.Error())
		}
		s.table = nil
	}
	if s.connection != nil {
		if err := s.connection.Close(); err != nil {
			errs = append(errs, err.Error())
		}
		s.connection = nil
	}
	if len(errs) > 0 {
		return fmt.Errorf("close LanceDB: %s", strings.Join(errs, "; "))
	}
	return nil
}

func matchesMetadata(metadata, filter map[string]string) bool {
	for key, value := range filter {
		if metadata[key] != value {
			return false
		}
	}
	return true
}

func escapeSQLLiteral(value string) string { return strings.ReplaceAll(value, "'", "''") }

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(value)
	}
}

func floatValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	default:
		return 0
	}
}

var _ VectorStore = (*LanceDBStore)(nil)
