package research

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/KyberixCo/Pharus/internal/llm"
)

type mockLLMProvider struct {
	response string
	err      error
}

func (m *mockLLMProvider) GenerateCompletion(ctx context.Context, messages []llm.Message, temperature float64) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestPlanBuilderFallback(t *testing.T) {
	pb := NewPlanBuilder(nil)
	plan, err := pb.BuildPlan(context.Background(), "Optimizaciones en Go y Llama.cpp", nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if plan.Topic != "Optimizaciones en Go y Llama.cpp" {
		t.Errorf("expected topic 'Optimizaciones en Go y Llama.cpp', got: %q", plan.Topic)
	}
	if plan.Audience != AudienceTechnical {
		t.Errorf("expected default technical audience, got: %q", plan.Audience)
	}
	if plan.Depth != DepthExhaustive {
		t.Errorf("expected default exhaustive depth, got: %q", plan.Depth)
	}
	if len(plan.Outline) < 4 {
		t.Errorf("expected at least 4 outline sections, got: %d", len(plan.Outline))
	}
	if err := plan.Validate(); err != nil {
		t.Errorf("expected plan validation to pass, got: %v", err)
	}
}

func TestPlanBuilderWithIntent(t *testing.T) {
	pb := NewPlanBuilder(nil)
	intent := &ResearchIntent{
		Audience:       AudienceExecutive,
		Depth:          DepthDeepDive,
		FocusAreas:     []string{"ROI", "Seguridad"},
		ExcludedTopics: []string{"Detalles de bajo nivel"},
	}
	plan, err := pb.BuildPlan(context.Background(), "Adopción de Inteligencia Artificial", intent)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if plan.Audience != AudienceExecutive {
		t.Errorf("expected executive audience, got: %q", plan.Audience)
	}
	if plan.Depth != DepthDeepDive {
		t.Errorf("expected deep dive depth, got: %q", plan.Depth)
	}
	if len(plan.RequiredAspects) < 5 {
		t.Errorf("expected required aspects to include focus areas, got: %v", plan.RequiredAspects)
	}
}

func TestPlanBuilderLLMSuccess(t *testing.T) {
	mockResponse := `{
		"topic": "Arquitectura de Agentes AI",
		"audience": "Technical",
		"depth": "Exhaustive",
		"core_hypothesis": "Los agentes multi-paso requieren planificación declarativa.",
		"required_aspects": ["Gobernanza", "Memoria"],
		"outline": [
			{
				"id": "sec_1",
				"title": "1. Introducción y Estado del Arte",
				"description": "Examen de arquitecturas modernas",
				"key_questions": ["¿Cuáles son los patrones principales?"],
				"sub_topics": ["Orquestación", "Memoria RAG"]
			},
			{
				"id": "sec_2",
				"title": "2. Evaluación de Rendimiento",
				"description": "Benchmarks empíricos",
				"key_questions": ["¿Qué métricas evalúan la latencia?"],
				"sub_topics": ["Latencia", "Throughput"]
			}
		]
	}`

	mockProvider := &mockLLMProvider{response: mockResponse}
	pb := NewPlanBuilder(mockProvider)
	plan, err := pb.BuildPlan(context.Background(), "Arquitectura de Agentes AI", nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(plan.Outline) != 2 {
		t.Errorf("expected 2 outline nodes from LLM, got: %d", len(plan.Outline))
	}
	if plan.Outline[0].Title != "1. Introducción y Estado del Arte" {
		t.Errorf("unexpected section title: %q", plan.Outline[0].Title)
	}
}

func TestPlanBuilderEmptyTopic(t *testing.T) {
	pb := NewPlanBuilder(nil)
	_, err := pb.BuildPlan(context.Background(), "   ", nil)
	if err == nil {
		t.Error("expected error for empty topic, got nil")
	}
}

func TestResearchPlanToJSON(t *testing.T) {
	pb := NewPlanBuilder(nil)
	plan, _ := pb.BuildPlan(context.Background(), "Test Topic", nil)
	jsonStr, err := plan.ToJSON()
	if err != nil {
		t.Fatalf("unexpected error converting to JSON: %v", err)
	}
	var unmarshaled ResearchPlan
	if err := json.Unmarshal([]byte(jsonStr), &unmarshaled); err != nil {
		t.Fatalf("unmarshaled invalid JSON: %v", err)
	}
}
