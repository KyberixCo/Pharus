package research

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/KyberixCo/Pharus/internal/llm"
	"github.com/KyberixCo/Pharus/internal/vectordb"
)

func TestValidateCitationsRejectsUnknownCitationAndInventedURL(t *testing.T) {
	sources := []CitationSource{{Number: 1, URL: "https://example.test/source", Title: "Source"}}
	report := "## Hallazgos\nUn hecho [99].\n\n## Referencias\n[99] Inventada — https://invented.test/law"
	if err := ValidateCitations(report, sources); err == nil {
		t.Fatal("expected invalid citation report to be rejected")
	}
}

func TestValidateCitationsAcceptsRecoveredSourceAndSnippetDisclosure(t *testing.T) {
	sources := []CitationSource{{Number: 1, URL: "https://example.test/source", Title: "Source", Type: EvidenceTypeSnippet}}
	report := "## Hallazgos\nLa evidencia es un snippet parcial y respalda este hecho [1].\n\n## Referencias\n[1] Source — https://example.test/source"
	if err := ValidateCitations(report, sources); err != nil {
		t.Fatalf("expected valid report, got %v", err)
	}
}

func TestValidateCitationsRejectsUncitedReference(t *testing.T) {
	sources := []CitationSource{{Number: 1, URL: "https://example.test/source", Title: "Source"}}
	report := "## Hallazgos\nSin afirmaciones factuales.\n\n## Referencias\n[1] Source — https://example.test/source"
	if err := ValidateCitations(report, sources); err == nil {
		t.Fatal("expected an unreferenced catalogue entry to be rejected")
	}
}

func TestValidateCitationsDoesNotTreatReferencesWordAsSectionHeading(t *testing.T) {
	sources := []CitationSource{{Number: 1, URL: "https://example.test/source", Title: "Source"}}
	report := "## Contexto\nEstas referencias históricas permiten sustentar adecuadamente el hallazgo descrito [1].\n\n## Referencias\n[1] Source — https://example.test/source"
	if err := ValidateCitations(report, sources); err != nil {
		t.Fatalf("expected references mentioned in prose not to truncate the report, got %v", err)
	}
}

func TestValidateCitationsFindsUncitedNestedSection(t *testing.T) {
	report := "## Análisis\n\n### Evidencia operativa\nEste apartado contiene una afirmación factual suficientemente extensa pero carece por completo de respaldo."
	err := ValidateCitations(report, nil)
	if err == nil || !strings.Contains(err.Error(), "Evidencia operativa") {
		t.Fatalf("expected nested uncited section to be identified, got %v", err)
	}
}

func TestValidateCitationsAllowsParentHeadingWithCitedChild(t *testing.T) {
	sources := []CitationSource{{Number: 1, URL: "https://example.test/source", Title: "Source"}}
	report := "## Análisis\n\n### Evidencia operativa\nEste apartado contiene una afirmación factual suficientemente extensa y respaldada [1].\n\n## Referencias\n[1] Source — https://example.test/source"
	if err := ValidateCitations(report, sources); err != nil {
		t.Fatalf("expected empty parent heading with cited child to pass, got %v", err)
	}
}

type sequentialSynthesisProvider struct {
	responses []string
	calls     int
}

func (p *sequentialSynthesisProvider) GenerateCompletion(_ context.Context, _ []llm.Message, _ float64) (string, error) {
	if len(p.responses) == 0 {
		return "", errors.New("no responses configured")
	}
	idx := p.calls
	if idx >= len(p.responses) {
		idx = len(p.responses) - 1
	}
	response := p.responses[idx]
	p.calls++
	return response, nil
}

func TestSynthesizerRepairsCitationFormatOnce(t *testing.T) {
	provider := &sequentialSynthesisProvider{responses: []string{
		"## Hallazgos\nHecho no sustentado [99].\n\n## Referencias\n[99] Inventada — https://invented.test",
		"## Hallazgos\nHecho recuperado [1].\n\n## Referencias\n[1] Fuente — https://example.test/source",
	}}
	singleNodeTree := &TaxmorphTree{
		Topic: "tema",
		Nodes: []*TaxonNode{{ID: "n1", Title: "Hallazgos", Level: 1}},
	}
	synth := NewSynthesizer(provider)
	report, err := synth.SynthesizeHierarchicalReport(context.Background(), "tema", singleNodeTree, []vectordb.SearchResult{{ID: "e1", Content: "contenido", Metadata: map[string]string{"url": "https://example.test/source", "title": "Fuente"}}}, nil, nil)
	if err != nil {
		t.Fatalf("expected repaired report, got %v", err)
	}
	if provider.calls != 2 {
		t.Fatalf("expected exactly one repair attempt, got %d calls", provider.calls)
	}
	if report == "" {
		t.Fatal("expected repaired report")
	}
}

func TestSynthesizerRejectsInvalidCitationsAfterRepair(t *testing.T) {
	provider := &sequentialSynthesisProvider{responses: []string{
		"## Hallazgos\nHecho [99].\n\n## Referencias\n[99] Inventada — https://invented.test",
		"## Hallazgos\nHecho [99].\n\n## Referencias\n[99] Inventada — https://invented.test",
	}}
	singleNodeTree := &TaxmorphTree{
		Topic: "tema",
		Nodes: []*TaxonNode{{ID: "n1", Title: "Hallazgos", Level: 1}},
	}
	_, err := NewSynthesizer(provider).SynthesizeHierarchicalReport(context.Background(), "tema", singleNodeTree, []vectordb.SearchResult{{ID: "e1", Content: "contenido", Metadata: map[string]string{"url": "https://example.test/source", "title": "Fuente"}}}, nil, nil)
	var citationErr *CitationValidationError
	if !errors.As(err, &citationErr) {
		t.Fatalf("expected CitationValidationError, got %v", err)
	}
	if provider.calls != 2 {
		t.Fatalf("expected exactly two calls, got %d", provider.calls)
	}
}
