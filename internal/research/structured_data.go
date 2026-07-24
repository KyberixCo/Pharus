package research

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"

	"github.com/KyberixCo/Pharus/internal/config"
)

var sqlTableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// StructuredDataProviderFromConfig loads the explicitly configured CSV files
// into DataSTORM's relational provider. An empty configuration keeps DataSTORM
// disabled; malformed or unreadable sources fail engine startup.
func StructuredDataProviderFromConfig(cfg *config.Config) (StructuredDataProvider, error) {
	if cfg == nil || len(cfg.DataSTORM.Sources) == 0 {
		return nil, nil
	}

	names := make([]string, 0, len(cfg.DataSTORM.Sources))
	for name := range cfg.DataSTORM.Sources {
		names = append(names, name)
	}
	sort.Strings(names)

	provider := NewInMemorySQLProvider()
	for _, name := range names {
		if !sqlTableNamePattern.MatchString(name) {
			return nil, fmt.Errorf("invalid DataSTORM table name %q", name)
		}
		path := cfg.DataSTORM.Sources[name]
		table, err := loadCSVTable(path)
		if err != nil {
			return nil, fmt.Errorf("load DataSTORM table %q: %w", name, err)
		}
		provider.AddTable(name, table)
	}
	return provider, nil
}

func loadCSVTable(path string) (SQLQueryResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return SQLQueryResult{}, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	columns, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			return SQLQueryResult{}, fmt.Errorf("CSV file is empty")
		}
		return SQLQueryResult{}, fmt.Errorf("read CSV header: %w", err)
	}
	if len(columns) == 0 {
		return SQLQueryResult{}, fmt.Errorf("CSV header has no columns")
	}

	rows := make([][]string, 0)
	for rowNumber := 2; ; rowNumber++ {
		row, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return SQLQueryResult{}, fmt.Errorf("read CSV row %d: %w", rowNumber, readErr)
		}
		if len(row) != len(columns) {
			return SQLQueryResult{}, fmt.Errorf("CSV row %d has %d fields; expected %d", rowNumber, len(row), len(columns))
		}
		rows = append(rows, row)
	}

	return SQLQueryResult{Columns: columns, Rows: rows, RowCount: len(rows)}, nil
}
