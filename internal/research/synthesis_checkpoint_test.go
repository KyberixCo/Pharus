package research

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSynthesisCheckpointRemapsCitationsWhenSourceOrderChanges(t *testing.T) {
	nodes := []*TaxonNode{{ID: "n1", Title: "Sección", Level: 1}}
	originalSources := []CitationSource{
		{Number: 1, URL: "https://example.test/a"},
		{Number: 2, URL: "https://example.test/b"},
	}
	ctx := withSynthesisCheckpoint(context.Background(), t.TempDir(), "tema", time.Hour)
	checkpoint := newSynthesisCheckpoint(nodes, originalSources)
	checkpoint.Sections["n1"] = "## Sección\n\nAfirmación suficientemente extensa respaldada por la segunda fuente recuperada [2]."
	if err := saveSynthesisCheckpoint(ctx, checkpoint, originalSources); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}

	reorderedSources := []CitationSource{
		{Number: 1, URL: "https://example.test/b"},
		{Number: 2, URL: "https://example.test/a"},
	}
	loaded, err := loadSynthesisCheckpoint(ctx, nodes, reorderedSources)
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	section, ok := loaded.reusableSection("n1", reorderedSources)
	if !ok {
		t.Fatal("expected checkpoint section to remain reusable")
	}
	if !strings.Contains(section, "[1]") || strings.Contains(section, "[2]") {
		t.Fatalf("expected citation to be remapped to new source number, got %q", section)
	}
}

func TestClearResearchSessionRemovesCompletedCheckpoints(t *testing.T) {
	baseDir := t.TempDir()
	ctx := withSynthesisCheckpoint(context.Background(), baseDir, "tema", time.Hour)
	nodes := []*TaxonNode{{ID: "n1", Title: "Sección", Level: 1}}
	sources := []CitationSource{{Number: 1, URL: "https://example.test/a"}}
	checkpoint := newSynthesisCheckpoint(nodes, sources)
	checkpoint.Sections["n1"] = "## Sección\n\nAfirmación suficientemente extensa respaldada por la fuente recuperada [1]."
	if err := saveSynthesisCheckpoint(ctx, checkpoint, sources); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}
	cfg, _ := checkpointConfigFromContext(ctx)
	if _, err := os.Stat(cfg.Path); err != nil {
		t.Fatalf("expected checkpoint file at %s: %v", filepath.Dir(cfg.Path), err)
	}
	if err := savePlanningCheckpoint(ctx, &ResearchPlan{Topic: "tema", Outline: []TopicNodeSpec{{Title: "Sección"}}}, &TaxmorphTree{Nodes: nodes}); err != nil {
		t.Fatalf("save planning checkpoint: %v", err)
	}
	if err := clearResearchSession(ctx); err != nil {
		t.Fatalf("clear checkpoint: %v", err)
	}
	if _, err := os.Stat(cfg.Path); !os.IsNotExist(err) {
		t.Fatalf("expected checkpoint to be removed, got %v", err)
	}
}
