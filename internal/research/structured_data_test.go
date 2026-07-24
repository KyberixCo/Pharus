package research

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KyberixCo/Pharus/internal/config"
)

func TestStructuredDataProviderFromConfigLoadsCSV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.csv")
	if err := os.WriteFile(path, []byte("topic,value\nlatency,42\nquality,98\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.DataSTORM.Sources = map[string]string{"research_metrics": path}

	provider, err := StructuredDataProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("configure DataSTORM: %v", err)
	}
	schema, err := provider.GetSchema(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(schema, "research_metrics") || !strings.Contains(schema, "topic, value") {
		t.Fatalf("unexpected schema: %s", schema)
	}
	result, err := provider.ExecuteSQL(context.Background(), "SELECT * FROM research_metrics")
	if err != nil {
		t.Fatal(err)
	}
	if result.RowCount != 2 || result.Rows[0][1] != "42" {
		t.Fatalf("unexpected query result: %+v", result)
	}
}

func TestStructuredDataProviderFromConfigRejectsInvalidSource(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DataSTORM.Sources = map[string]string{"bad-name;drop": "/missing.csv"}
	if _, err := StructuredDataProviderFromConfig(cfg); err == nil {
		t.Fatal("expected invalid table name to fail")
	}
}
