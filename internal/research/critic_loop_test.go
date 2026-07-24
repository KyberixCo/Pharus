package research

import (
	"context"
	"strings"
	"testing"

	"github.com/KyberixCo/Pharus/internal/llm"
	"github.com/philippgille/chromem-go"
)

type mockCriticLLM struct {
	evaluateResp string
	refineResp   string
	evaluateErr  error
	refineErr    error
	evalCalls    int
	refineCalls  int
}

func (m *mockCriticLLM) GenerateCompletion(ctx context.Context, messages []llm.Message, temperature float64) (string, error) {
	for _, msg := range messages {
		if strings.Contains(msg.Content, "INSTRUCCIONES DE EVALUACIÓN") {
			m.evalCalls++
			if m.evaluateErr != nil {
				return "", m.evaluateErr
			}
			return m.evaluateResp, nil
		}
		if strings.Contains(msg.Content, "INSTRUCCIONES DE REESCRITURA LOCALIZADA") {
			m.refineCalls++
			if m.refineErr != nil {
				return "", m.refineErr
			}
			return m.refineResp, nil
		}
	}
	return "PASS", nil
}

type mockEmbedder struct {
	embeddings map[string][]float32
}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if emb, ok := m.embeddings[text]; ok {
		return emb, nil
	}
	return []float32{1.0, 0.0, 0.0}, nil
}

func (m *mockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	res := make([][]float32, len(texts))
	for i, t := range texts {
		emb, _ := m.Embed(ctx, t)
		res[i] = emb
	}
	return res, nil
}

func (m *mockEmbedder) ChromemEmbeddingFunc() chromem.EmbeddingFunc {
	return func(ctx context.Context, text string) ([]float32, error) {
		return m.Embed(ctx, text)
	}
}

func TestEvaluateReportParsing(t *testing.T) {
	resp := `COHESION_PASSED: false
CONSISTENCY_PASSED: true
RELEVANCE_DEPTH_PASSED: false
OVERALL_PASSED: false
FLAWED_SECTIONS: ## 2. Arquitectura, ## 3. Evaluación
INSTRUCTIONS: Ampliar el desarrollo técnico y mejorar las transiciones entre secciones.`

	res := parseRubricResponse(resp)

	if res.CohesionPassed {
		t.Errorf("expected CohesionPassed to be false")
	}
	if !res.ConsistencyPassed {
		t.Errorf("expected ConsistencyPassed to be true")
	}
	if res.RelevanceDepthPassed {
		t.Errorf("expected RelevanceDepthPassed to be false")
	}
	if res.OverallPassed {
		t.Errorf("expected OverallPassed to be false")
	}
	if len(res.FlawedSections) != 2 {
		t.Fatalf("expected 2 flawed sections, got %d", len(res.FlawedSections))
	}
	if res.FlawedSections[0] != "## 2. Arquitectura" || res.FlawedSections[1] != "## 3. Evaluación" {
		t.Errorf("unexpected flawed sections: %v", res.FlawedSections)
	}
	if res.Instructions == "" {
		t.Errorf("expected non-empty instructions")
	}
}

func TestCriticLoopNilLLM(t *testing.T) {
	critic := NewCriticLoop(nil, nil, CriticConfig{})
	sources := []CitationSource{{Number: 1, URL: "https://example.com", Title: "Fuente 1"}}

	report, status, warnings, err := critic.RunCriticLoop(context.Background(), "Topic", "# Informe Inicial", sources)
	if err != nil {
		t.Fatalf("expected no error with nil LLM, got %v", err)
	}
	if status != ResearchStatusSuccess {
		t.Errorf("expected success status, got %s", status)
	}
	if report != "# Informe Inicial" {
		t.Errorf("expected unchanged report, got %s", report)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestCriticLoopSuccessFirstPass(t *testing.T) {
	mockLLM := &mockCriticLLM{
		evaluateResp: `COHESION_PASSED: true
CONSISTENCY_PASSED: true
RELEVANCE_DEPTH_PASSED: true
OVERALL_PASSED: true
FLAWED_SECTIONS: Ninguna
INSTRUCTIONS: Ninguna`,
	}

	critic := NewCriticLoop(mockLLM, nil, CriticConfig{MaxIterations: 2})
	sources := []CitationSource{{Number: 1, URL: "https://example.com", Title: "Fuente 1"}}

	initialReport := "# Informe Completo\n\nDesarrollo extenso [1].\n\n## Referencias\n\n[1] Fuente 1 — https://example.com\n"
	report, status, warnings, err := critic.RunCriticLoop(context.Background(), "Topic", initialReport, sources)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if status != ResearchStatusSuccess {
		t.Errorf("expected success status, got %s", status)
	}
	if report != initialReport {
		t.Errorf("expected initial report to be returned, got %s", report)
	}
	if mockLLM.evalCalls != 1 {
		t.Errorf("expected 1 evaluation call, got %d", mockLLM.evalCalls)
	}
	if mockLLM.refineCalls != 0 {
		t.Errorf("expected 0 refinement calls when evaluation passes, got %d", mockLLM.refineCalls)
	}
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings, got %v", warnings)
	}
}

func TestCriticLoopRefinementAndConvergence(t *testing.T) {
	refinedReport := "# Informe Refinado\n\nDesarrollo técnico profundo [1].\n\n## Referencias\n\n[1] Fuente 1 — https://example.com\n"

	mockLLM := &mockCriticLLM{
		evaluateResp: `COHESION_PASSED: false
CONSISTENCY_PASSED: true
RELEVANCE_DEPTH_PASSED: false
OVERALL_PASSED: false
FLAWED_SECTIONS: ## 1. Introducción
INSTRUCTIONS: Profundizar en los detalles técnicos.`,
		refineResp: refinedReport,
	}

	// Mock embedder returning identical embeddings to trigger convergence threshold (sim = 1.0 >= 0.96)
	mockEmb := &mockEmbedder{
		embeddings: map[string][]float32{
			"# Informe Inicial": {1.0, 0.0, 0.0},
			refinedReport:       {1.0, 0.0, 0.0},
		},
	}

	critic := NewCriticLoop(mockLLM, mockEmb, CriticConfig{
		MaxIterations:        2,
		ConvergenceThreshold: 0.96,
	})
	sources := []CitationSource{{Number: 1, URL: "https://example.com", Title: "Fuente 1"}}

	initialReport := "# Informe Inicial\n\nTexto breve [1].\n\n## Referencias\n\n[1] Fuente 1 — https://example.com\n"
	report, status, warnings, err := critic.RunCriticLoop(context.Background(), "Topic", initialReport, sources)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if status != ResearchStatusSuccess {
		t.Errorf("expected status success, got %s", status)
	}
	if report != refinedReport {
		t.Errorf("expected refined report, got:\n%s", report)
	}
	if mockLLM.refineCalls != 1 {
		t.Errorf("expected 1 refinement call, got %d", mockLLM.refineCalls)
	}

	hasConvergenceWarning := false
	for _, w := range warnings {
		if strings.Contains(w, "umbral de convergencia") {
			hasConvergenceWarning = true
			break
		}
	}
	if !hasConvergenceWarning {
		t.Errorf("expected convergence warning, got %v", warnings)
	}
}

func TestCriticLoopMaxIterationsDegraded(t *testing.T) {
	refinedReport := "# Informe Refinado\n\nTexto mejorado [1].\n\n## Referencias\n\n[1] Fuente 1 — https://example.com\n"

	mockLLM := &mockCriticLLM{
		evaluateResp: `COHESION_PASSED: false
CONSISTENCY_PASSED: true
RELEVANCE_DEPTH_PASSED: false
OVERALL_PASSED: false
FLAWED_SECTIONS: ## 1. Introducción
INSTRUCTIONS: Falta profundidad.`,
		refineResp: refinedReport,
	}

	// Embedder returns orthogonal vectors to prevent early convergence
	callCount := 0
	mockEmbFunc := func(ctx context.Context, text string) ([]float32, error) {
		callCount++
		if callCount == 1 {
			return []float32{1.0, 0.0, 0.0}, nil
		}
		return []float32{0.0, 1.0, 0.0}, nil
	}

	_ = mockEmbFunc // embedder provided via struct below

	critic := NewCriticLoop(mockLLM, nil, CriticConfig{
		MaxIterations:        1,
		ConvergenceThreshold: 0.96,
	})
	sources := []CitationSource{{Number: 1, URL: "https://example.com", Title: "Fuente 1"}}

	initialReport := "# Informe Inicial\n\nTexto breve [1].\n\n## Referencias\n\n[1] Fuente 1 — https://example.com\n"
	_, status, warnings, err := critic.RunCriticLoop(context.Background(), "Topic", initialReport, sources)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if status != ResearchStatusDegraded {
		t.Errorf("expected degraded status after max iterations failure, got %s", status)
	}

	hasDegradedWarning := false
	for _, w := range warnings {
		if strings.Contains(w, "agotando las iteraciones") || strings.Contains(w, "agotar las iteraciones") {
			hasDegradedWarning = true
			break
		}
	}
	if !hasDegradedWarning {
		t.Errorf("expected degraded warning, got %v", warnings)
	}
}
