package research

import (
	"context"
	"strings"
	"testing"
)

func TestConceptMapAndDiscourseManager(t *testing.T) {
	cm := NewConceptMap()
	cm.AddOrUpdateNode("node_1", "Deep Research", "Pharus es un motor en Go", "Falta soporte OOM")
	cm.AddSubConcept("node_1", "Co-STORM")
	cm.ResolveGap("node_1", "Falta soporte OOM")

	summary := cm.Summary()
	if !strings.Contains(summary, "node_1") {
		t.Errorf("expected summary to contain node_1")
	}
	if !strings.Contains(summary, "Sub-conceptos: Co-STORM") {
		t.Errorf("expected summary to contain sub-concepts")
	}

	dm := NewDiscourseManager(nil)
	ctx := context.Background()

	dm.UpdateConceptMapFromEvidence(ctx, "Deep Research", "Título: Pharus Architecture\nContenido: Go backend with LanceDB")
	dmSummary := dm.ConceptMap.Summary()
	if !strings.Contains(dmSummary, "Pharus Architecture") && !strings.Contains(dmSummary, "deep_research") {
		t.Errorf("expected ConceptMap to update from evidence text")
	}
}

func TestDiscourseManagerPerspectivesFallback(t *testing.T) {
	dm := NewDiscourseManager(nil)
	ctx := context.Background()

	roles, err := dm.GeneratePerspectives(ctx, "Vector DB Performance")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) == 0 {
		t.Fatalf("expected default expert roles when LLM is nil")
	}

	provocations, err := dm.EvaluateGapsAndProvoke(ctx, "Vector DB Performance", "Evidence summary text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(provocations) == 0 {
		t.Fatalf("expected default provocative questions when LLM is nil")
	}
}
