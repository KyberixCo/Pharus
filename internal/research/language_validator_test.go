package research

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/KyberixCo/Pharus/internal/vectordb"
)

func TestValidateReportLanguageRejectsMixedChineseCharacters(t *testing.T) {
	report := "## Refrigeración\n\ny a partir de ahí sucesivas etapas de稀释 refrigeración alcanzan temperaturas menores [1]."
	var languageErr *LanguageValidationError
	if err := ValidateReportLanguage(report); !errors.As(err, &languageErr) {
		t.Fatalf("expected mixed Chinese characters to be rejected, got %v", err)
	}
}

func TestValidateReportLanguageAcceptsSpanishProse(t *testing.T) {
	report := "## Refrigeración\n\nA partir de ahí, sucesivas etapas de refrigeración por dilución alcanzan temperaturas menores [1]."
	if err := ValidateReportLanguage(report); err != nil {
		t.Fatalf("expected Spanish prose to pass, got %v", err)
	}
}

func TestValidateReportLanguageAllowsOriginalScriptInReferences(t *testing.T) {
	report := "## Hallazgos\n\nLa fuente documenta el resultado experimental [1].\n\n## Referencias\n[1] 量子计算研究 — https://example.test/source"
	if err := ValidateReportLanguage(report); err != nil {
		t.Fatalf("expected original source title in references to pass, got %v", err)
	}
}

func TestSynthesizerRepairsMixedChineseCharactersLocally(t *testing.T) {
	provider := &sequentialSynthesisProvider{responses: []string{
		"## Refrigeración\n\nA partir de ahí, sucesivas etapas de稀释 refrigeración alcanzan temperaturas menores [1].",
		"## Refrigeración\n\nA partir de ahí, sucesivas etapas de refrigeración por dilución alcanzan temperaturas menores [1].",
	}}
	tree := &TaxmorphTree{Nodes: []*TaxonNode{{ID: "n1", Title: "Refrigeración", Level: 1}}}
	evidence := []vectordb.SearchResult{{
		ID:       "e1",
		Content:  "La refrigeración por dilución permite alcanzar temperaturas muy bajas.",
		Metadata: map[string]string{"url": "https://example.test/source", "title": "Fuente"},
	}}

	report, err := NewSynthesizer(provider).SynthesizeHierarchicalReport(context.Background(), "Tema", tree, evidence, nil, nil)
	if err != nil {
		t.Fatalf("expected localized language repair, got %v", err)
	}
	if provider.calls != 2 {
		t.Fatalf("expected one section repair, got %d calls", provider.calls)
	}
	if strings.Contains(report, "稀释") || !strings.Contains(report, "dilución") {
		t.Fatalf("expected a Spanish-only repaired report, got:\n%s", report)
	}
}
