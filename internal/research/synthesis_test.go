package research

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/KyberixCo/Pharus/internal/llm"
	"github.com/KyberixCo/Pharus/internal/vectordb"
)

type mockSynthesizerLLM struct {
	responses map[string]string
	callCount int
	err       error
}

func (m *mockSynthesizerLLM) GenerateCompletion(ctx context.Context, messages []llm.Message, temperature float64) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	m.callCount++
	var allContent strings.Builder
	for _, msg := range messages {
		allContent.WriteString(msg.Content)
		allContent.WriteString("\n")
	}
	fullText := allContent.String()

	if strings.Contains(fullText, "Sección a Redactar: ### 2. Arquitectura") || strings.Contains(fullText, "Sección a Redactar: ## 2. Arquitectura") {
		return "### 2. Arquitectura\n\nContinuando con la introducción previa sobre GBNF, la arquitectura desacopla el planificador de consultas de LanceDB [1].", nil
	}
	if strings.Contains(fullText, "Sección a Redactar: ## 1. Introducción") {
		return "## 1. Introducción\n\nAnalizamos el motor Pharus y sus capacidades de inferencia en llama.cpp [1]. Esta sección introduce los conceptos de GBNF.", nil
	}

	for key, resp := range m.responses {
		if strings.Contains(fullText, key) {
			return resp, nil
		}
	}

	return "## Resumen\n\nEste apartado desarrolla en profundidad el tema analizando la evidencia técnica [1]. Se evalúan los componentes clave de GBNF y LLM.", nil
}

func TestSynthesizeHierarchicalReportFallback(t *testing.T) {
	synth := NewSynthesizer(nil)
	evidence := []vectordb.SearchResult{
		{
			ID:      "doc1",
			Content: "Go es un lenguaje concurrente y rápido.",
			Metadata: map[string]string{
				"url":   "https://golang.org",
				"title": "Go Documentation",
			},
		},
	}

	tree := &TaxmorphTree{
		Nodes: []*TaxonNode{
			{
				ID:          "sec_1",
				Title:       "1. Introducción",
				Description: "Visión general de la tecnología",
				Level:       1,
			},
		},
	}

	report, err := synth.SynthesizeHierarchicalReport(context.Background(), "Go Optimization", tree, evidence, nil, nil)
	if err != nil {
		t.Fatalf("expected no error in fallback synthesis, got: %v", err)
	}

	if !strings.Contains(report, "# Informe de Deep Research: Go Optimization") {
		t.Errorf("expected report title, got:\n%s", report)
	}
	if !strings.Contains(report, "## Referencias") {
		t.Errorf("expected references section, got:\n%s", report)
	}
	if !strings.Contains(report, "[1] Go Documentation — https://golang.org") {
		t.Errorf("expected citation line, got:\n%s", report)
	}
}

func TestSynthesizeHierarchicalReportMockLLM(t *testing.T) {
	mockLLM := &mockSynthesizerLLM{}

	synth := NewSynthesizer(mockLLM)
	evidence := []vectordb.SearchResult{
		{
			ID:      "doc1",
			Content: "Pharus implementa inferencia local con gramáticas GBNF.",
			Metadata: map[string]string{
				"url":   "https://pharus.ai/docs",
				"title": "Pharus Docs",
			},
		},
	}

	tree := &TaxmorphTree{
		Nodes: []*TaxonNode{
			{
				ID:          "sec_1",
				Title:       "1. Introducción",
				Description: "Visión general",
				Level:       1,
			},
			{
				ID:          "sec_2",
				Title:       "2. Arquitectura",
				Description: "Detalles del sistema",
				Level:       2,
			},
		},
	}

	report, err := synth.SynthesizeHierarchicalReport(context.Background(), "Pharus Deep Research", tree, evidence, nil, nil)
	if err != nil {
		t.Fatalf("expected successful synthesis, got: %v", err)
	}

	if !strings.Contains(report, "## 1. Introducción") {
		t.Errorf("missing section 1 in report:\n%s", report)
	}
	if !strings.Contains(report, "### 2. Arquitectura") {
		t.Errorf("missing section 2 in report:\n%s", report)
	}
	if mockLLM.callCount < 2 {
		t.Errorf("expected at least 2 LLM section calls, got: %d", mockLLM.callCount)
	}
}

func TestSynthesizeReferencesOnlyCitedSources(t *testing.T) {
	mockLLM := &mockSynthesizerLLM{}
	evidence := []vectordb.SearchResult{
		{ID: "doc1", Content: "Evidencia principal.", Metadata: map[string]string{"url": "https://example.com/one", "title": "Fuente Uno"}},
		{ID: "doc2", Content: "Evidencia secundaria.", Metadata: map[string]string{"url": "https://example.com/two", "title": "Fuente Dos"}},
	}
	tree := &TaxmorphTree{Nodes: []*TaxonNode{{ID: "sec_1", Title: "Resumen", Description: "Síntesis", Level: 1}}}

	report, err := NewSynthesizer(mockLLM).SynthesizeHierarchicalReport(context.Background(), "Tema", tree, evidence, nil, nil)
	if err != nil {
		t.Fatalf("expected synthesis using a subset of sources to pass, got %v", err)
	}
	if !strings.Contains(report, "[1] Fuente Uno — https://example.com/one") {
		t.Fatalf("expected cited source in references, got:\n%s", report)
	}
	if strings.Contains(report, "[2] Fuente Dos — https://example.com/two") {
		t.Fatalf("did not expect uncited source in references, got:\n%s", report)
	}
}

func TestSlidingContextActiveEntities(t *testing.T) {
	initial := []string{"GBNF"}
	sampleText := "La integración de llama.cpp con LanceDB y DataSTORM ofrece alta resiliencia en Pharus."

	updated := updateActiveEntities(initial, sampleText)

	expected := map[string]bool{
		"GBNF":      true,
		"DataSTORM": true,
		"LanceDB":   true,
		"Pharus":    true,
	}

	for exp := range expected {
		found := false
		for _, u := range updated {
			if u == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected entity %q in updated active entities, got: %v", exp, updated)
		}
	}
}

func TestExtractTailParagraphs(t *testing.T) {
	text := `
# Encabezado

Primer párrafo de la sección.

Segundo párrafo importante sobre la arquitectura de Pharus.

Tercer párrafo conclusivo que sirve como puente de transición.
`
	tail := extractTailParagraphs(text, 2)
	if !strings.Contains(tail, "Segundo párrafo") || !strings.Contains(tail, "Tercer párrafo") {
		t.Errorf("unexpected tail paragraphs extracted: %q", tail)
	}
}

func TestSynthesizeWithoutSourcesError(t *testing.T) {
	synth := NewSynthesizer(nil)
	tree := &TaxmorphTree{Nodes: []*TaxonNode{{ID: "1", Title: "Title"}}}
	_, err := synth.SynthesizeHierarchicalReport(context.Background(), "Topic", tree, nil, nil, nil)
	if err == nil {
		t.Error("expected error when evidence has no sources, got nil")
	}
}

func TestRepairCitationsSuccess(t *testing.T) {
	mockLLM := &mockSynthesizerLLM{
		responses: map[string]string{
			"Reparas citas": "## Introducción\n\nTexto corregido con cita válida [1].\n\n## Referencias\n\n[1] Fuente Uno — https://example.com\n",
		},
	}
	synth := NewSynthesizer(mockLLM)
	sources := []CitationSource{{Number: 1, ID: "d1", URL: "https://example.com", Title: "Fuente Uno"}}
	repaired, err := synth.repairCitations(context.Background(), "# Original", sources, fmt.Errorf("citas inválidas [99]"))
	if err != nil {
		t.Fatalf("expected repair to succeed, got: %v", err)
	}
	if !strings.Contains(repaired, "[1] Fuente Uno — https://example.com") {
		t.Errorf("repaired text missing reference: %s", repaired)
	}
}
