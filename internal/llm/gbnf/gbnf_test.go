package gbnf

import (
	"strings"
	"testing"
)

func TestGBNFGenerator_GenerateObjectGrammar(t *testing.T) {
	gen := NewGBNFGenerator()
	grammar := gen.GenerateObjectGrammar(map[string]string{
		"topic":   "string",
		"score":   "number",
		"active":  "boolean",
		"tags":    "string-array",
		"status":  "enum:draft,published,archived",
	})

	if !strings.Contains(grammar, "root ::=") {
		t.Errorf("expected grammar to contain root definition")
	}
	if !strings.Contains(grammar, "topic") {
		t.Errorf("expected topic field in grammar")
	}
	if !strings.Contains(grammar, "draft") || !strings.Contains(grammar, "published") {
		t.Errorf("expected enum options in grammar, got:\n%s", grammar)
	}
}

func TestGBNFGenerator_ToolCallAndCoSTORM(t *testing.T) {
	gen := NewGBNFGenerator()
	
	toolGrammar := gen.GenerateToolCallGrammar("search_web", map[string]string{
		"query": "string",
		"count": "number",
	})
	if !strings.Contains(toolGrammar, "search_web") || !strings.Contains(toolGrammar, "tool") {
		t.Errorf("expected tool call grammar format, got:\n%s", toolGrammar)
	}

	costormGrammar := gen.GenerateCoSTORMQuestionsGrammar()
	if !strings.Contains(costormGrammar, "perspective") || !strings.Contains(costormGrammar, "questions") {
		t.Errorf("expected Co-STORM grammar format, got:\n%s", costormGrammar)
	}
}


func TestGBNFGenerator_MaskLogits(t *testing.T) {
	gen := NewGBNFGenerator()
	logits := []float32{0.5, 1.2, 0.3, 2.5}
	masked := gen.MaskLogits(logits, []int{1, 3})

	if masked[0] != -1e9 || masked[2] != -1e9 {
		t.Errorf("expected unselected tokens to be masked out")
	}

	if masked[1] != 1.2 || masked[3] != 2.5 {
		t.Errorf("expected selected tokens to retain original logits")
	}
}

