package research

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/KyberixCo/Pharus/internal/llm"
)

// RACEScore holds the numerical grades (0.0 to 1.0) evaluated across the 4 RACE criteria.
type RACEScore struct {
	Relevance    float64 `json:"relevance"`    // Relevancia temática del contenido
	Authenticity float64 `json:"authenticity"` // Autenticidad y fidelidad de las evidencias/citas
	Clarity      float64 `json:"clarity"`      // Claridad de estructura y coherencia discursiva
	Evidence     float64 `json:"evidence"`     // Cobertura y profundidad empírica de evidencia
	OverallRACE  float64 `json:"overall_race"` // Promedio compuesto RACE
}

type RACEEvaluator struct {
	llm llm.Provider
}

var raceScorePattern = regexp.MustCompile(`(?im)^\s*(Relevance|Authenticity|Clarity|Evidence)\s*:\s*([01](?:\.\d+)?)\s*$`)

func NewRACEEvaluator(llmClient llm.Provider) *RACEEvaluator {
	return &RACEEvaluator{llm: llmClient}
}

// EvaluateReport performs LLM-as-a-judge evaluation of a markdown report based on the RACE rubric.
func (e *RACEEvaluator) EvaluateReport(ctx context.Context, topic string, report string) (*RACEScore, error) {
	if e == nil || e.llm == nil {
		return nil, fmt.Errorf("RACE evaluator requires an LLM provider")
	}
	if strings.TrimSpace(topic) == "" {
		return nil, fmt.Errorf("RACE evaluation requires a topic")
	}
	if strings.TrimSpace(report) == "" {
		return nil, fmt.Errorf("RACE evaluation requires a non-empty report")
	}
	promptTemplate := researchText(ctx, `Act as an expert LLM-as-a-judge evaluator using the RACE Score rubric (Relevance, Authenticity, Clarity, Evidence).
Evaluate the following Deep Research report for the topic: "%s"

Report to evaluate:
%s

Assign a score from 0.0 to 1.0 for each criterion:
1. Relevance (direct topical relevance)
2. Authenticity (valid citations and absence of hallucinations)
3. Clarity (excellent Markdown structure and discourse flow)
4. Evidence (coverage and depth of collected empirical evidence)

Return exactly this line-oriented format:
Relevance: <num>
Authenticity: <num>
Clarity: <num>
Evidence: <num>`, `Actúa como evaluador experto (LLM-as-a-judge) bajo la rúbrica RACE Score (Relevance, Authenticity, Clarity, Evidence).
Evalúa el siguiente reporte de Deep Research para el tema: "%s"

Reporte a evaluar:
%s

Asigna una puntuación de 0.0 a 1.0 para cada criterio:
1. Relevance (Relevancia temática directa)
2. Authenticity (Citas válidas y ausencia de alucinaciones)
3. Clarity (Estructura Markdown impecable y fluidez discursiva)
4. Evidence (Soporte de evidencias empíricas recolectadas)

Devuelve exactamente en este formato por línea:
Relevance: <num>
Authenticity: <num>
Clarity: <num>
Evidence: <num>`)
	prompt := fmt.Sprintf(promptTemplate, topic, report)

	messages := []llm.Message{
		{Role: "system", Content: researchText(ctx,
			"You are a rigorous evaluator of scientific Deep Research reports using the RACE Score standard.",
			"Eres un evaluador riguroso de reportes científicos de Deep Research utilizando el estándar RACE Score.",
		) + "\n\n" + reportLanguageDirective(ctx)},
		{Role: "user", Content: prompt},
	}

	resp, err := e.llm.GenerateCompletion(ctx, messages, 0.1)
	if err != nil {
		return nil, fmt.Errorf("generate RACE evaluation: %w", err)
	}

	values := make(map[string]float64, 4)
	for _, match := range raceScorePattern.FindAllStringSubmatch(resp, -1) {
		value, parseErr := strconv.ParseFloat(match[2], 64)
		if parseErr != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
			return nil, fmt.Errorf("invalid RACE %s score %q", strings.ToLower(match[1]), match[2])
		}
		key := strings.ToLower(match[1])
		if _, duplicate := values[key]; duplicate {
			return nil, fmt.Errorf("duplicate RACE %s score", key)
		}
		values[key] = value
	}
	for _, criterion := range []string{"relevance", "authenticity", "clarity", "evidence"} {
		if _, ok := values[criterion]; !ok {
			return nil, fmt.Errorf("invalid RACE evaluation response: missing %s score", criterion)
		}
	}

	score := &RACEScore{
		Relevance: values["relevance"], Authenticity: values["authenticity"],
		Clarity: values["clarity"], Evidence: values["evidence"],
	}
	score.OverallRACE = (score.Relevance + score.Authenticity + score.Clarity + score.Evidence) / 4.0
	return score, nil
}
