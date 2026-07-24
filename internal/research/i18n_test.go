package research

import (
	"context"
	"strings"
	"testing"

	"github.com/KyberixCo/Pharus/internal/i18n"
	"github.com/KyberixCo/Pharus/internal/llm"
)

type languageCapturingProvider struct {
	messages []llm.Message
	response string
}

func (p *languageCapturingProvider) GenerateCompletion(_ context.Context, messages []llm.Message, _ float64) (string, error) {
	p.messages = append([]llm.Message(nil), messages...)
	return p.response, nil
}

func TestPlanBuilderUsesSelectedEnglishInPrompts(t *testing.T) {
	provider := &languageCapturingProvider{response: `{
		"topic":"Quantum computing",
		"audience":"Technical",
		"depth":"Exhaustive",
		"core_hypothesis":"Quantum advantage can be measured.",
		"required_aspects":["Evidence"],
		"outline":[{"id":"section_1","title":"1. Evidence","description":"Review evidence","key_questions":["What is demonstrated?"],"sub_topics":["Benchmarks"]}]
	}`}
	ctx := i18n.WithLanguage(context.Background(), i18n.English)
	if _, err := NewPlanBuilder(provider).BuildPlan(ctx, "Quantum computing", nil); err != nil {
		t.Fatal(err)
	}
	if len(provider.messages) != 2 {
		t.Fatalf("expected two prompt messages, got %d", len(provider.messages))
	}
	combined := provider.messages[0].Content + "\n" + provider.messages[1].Content
	for _, expected := range []string{"REQUIRED LANGUAGE", "professional English", "Research Topic", "GENERATION RULES"} {
		if !strings.Contains(combined, expected) {
			t.Fatalf("English prompt is missing %q:\n%s", expected, combined)
		}
	}
	if strings.Contains(combined, "Tema de Investigación") || strings.Contains(combined, "IDIOMA OBLIGATORIO") {
		t.Fatalf("English prompt leaked Spanish instructions:\n%s", combined)
	}
}

func TestPlanBuilderFallbackUsesSelectedEnglish(t *testing.T) {
	ctx := i18n.WithLanguage(context.Background(), i18n.English)
	plan, err := NewPlanBuilder(nil).BuildPlan(ctx, "Quantum computing", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Outline) == 0 || !strings.Contains(plan.Outline[0].Title, "Executive Summary") {
		t.Fatalf("expected an English fallback outline, got %#v", plan.Outline)
	}
}

func TestSynthesizerUsesSelectedEnglishInSectionPrompt(t *testing.T) {
	provider := &languageCapturingProvider{response: "## Evidence\n\nSupported finding [1]."}
	ctx := i18n.WithLanguage(context.Background(), i18n.English)
	node := &TaxonNode{
		ID:           "evidence",
		Title:        "Evidence",
		Description:  "Assess the available evidence",
		Level:        1,
		KeyQuestions: []string{"What is supported?"},
	}
	_, err := NewSynthesizer(provider).synthesizeNodeSection(
		ctx, "Quantum computing", node, &SlidingContext{},
		"[1] URL: https://example.test\nTitle: Source\nRetrieved content:\nEvidence",
		"Concept map", "No additional findings.", "2025: Event", 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	combined := provider.messages[0].Content + "\n" + provider.messages[1].Content
	for _, expected := range []string{"professional English", "Global Topic", "STRICT WRITING AND CITATION INSTRUCTIONS"} {
		if !strings.Contains(combined, expected) {
			t.Fatalf("English synthesis prompt is missing %q:\n%s", expected, combined)
		}
	}
	if strings.Contains(combined, "Tema Global") || strings.Contains(combined, "INSTRUCCIONES ESTRICTAS") {
		t.Fatalf("English synthesis prompt leaked Spanish instructions:\n%s", combined)
	}
}

func TestLanguageSpecificSessionKeysDoNotCollide(t *testing.T) {
	topic := "Same topic"
	english := languageSessionTopic(topic, i18n.English)
	spanish := languageSessionTopic(topic, i18n.Spanish)
	if english == spanish {
		t.Fatal("English and Spanish session keys must differ")
	}
}
