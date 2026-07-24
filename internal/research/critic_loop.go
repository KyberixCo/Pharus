package research

import (
	"context"
	"fmt"
	"strings"

	"github.com/KyberixCo/Pharus/internal/embedding"
	"github.com/KyberixCo/Pharus/internal/llm"
	"github.com/KyberixCo/Pharus/internal/vectordb"
	"github.com/KyberixCo/Pharus/pkg/logger"
)

// ReinhartRubricResult contains the quantitative and qualitative outcome
// of evaluating a report draft against Reinhart's discourse coherence dimensions.
type ReinhartRubricResult struct {
	CohesionPassed       bool     `json:"cohesion_passed"`
	ConsistencyPassed    bool     `json:"consistency_passed"`
	RelevanceDepthPassed bool     `json:"relevance_depth_passed"`
	OverallPassed        bool     `json:"overall_passed"`
	FlawedSections       []string `json:"flawed_sections"`
	Instructions         string   `json:"instructions"`
}

// CriticConfig controls the loop engineering parameters for draft refinement.
type CriticConfig struct {
	MaxIterations        int     // Maximum revision pases (default 2)
	ConvergenceThreshold float64 // Cosine similarity threshold to stop early (default 0.96)
}

// CriticLoop implements Phase 6: Critic-Refinement Loop Governance (Loop Engineering).
type CriticLoop struct {
	llm    llm.Provider
	embed  embedding.Provider
	config CriticConfig
}

// NewCriticLoop creates a new CriticLoop governance engine.
func NewCriticLoop(llmProvider llm.Provider, embedProvider embedding.Provider, cfg CriticConfig) *CriticLoop {
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = 2
	}
	if cfg.ConvergenceThreshold <= 0 {
		cfg.ConvergenceThreshold = 0.96
	}
	return &CriticLoop{
		llm:    llmProvider,
		embed:  embedProvider,
		config: cfg,
	}
}

// EvaluateReport checks the draft against the three Reinhart discourse dimensions:
// 1. Cohesion (transitions, unequivocal pronouns)
// 2. Consistency (no internal factual contradictions)
// 3. Relevance & Depth (exhaustive coverage without superficial bullet lists)
func (c *CriticLoop) EvaluateReport(ctx context.Context, topic string, report string) (*ReinhartRubricResult, error) {
	if c.llm == nil {
		return &ReinhartRubricResult{
			CohesionPassed:       true,
			ConsistencyPassed:    true,
			RelevanceDepthPassed: true,
			OverallPassed:        true,
		}, nil
	}

	promptTemplate := researchText(ctx, `Research topic: %q

REPORT TO EVALUATE:
%s

EVALUATION INSTRUCTIONS (Reinhart Discourse Criteria):
Evaluate the draft carefully along three dimensions:
1. COHESION: Are transitions between sections fluid? Are pronouns resolved clearly?
2. CONSISTENCY: Is it free of internal factual contradictions and anachronisms?
3. RELEVANCE AND DEPTH: Are topics developed in depth (at least 300 words per section, without empty bullet lists or brief synthetic paragraphs)?

RETURN THE RESULT WITH THESE EXACT KEYS (one per line):
COHESION_PASSED: true|false
CONSISTENCY_PASSED: true|false
RELEVANCE_DEPTH_PASSED: true|false
OVERALL_PASSED: true|false
FLAWED_SECTIONS: Comma-separated section titles with deficiencies (or "None")
INSTRUCTIONS: Concrete instructions to fix the identified issues (or "None")`, `Tema de la investigación: %q

INFORME A EVALUAR:
%s

INSTRUCCIONES DE EVALUACIÓN (Criterios Discursivos de Reinhart):
Evalúa minuciosamente el borrador anterior en tres dimensiones:
1. COHESIÓN: ¿Existen transiciones fluidas entre secciones? ¿Se resuelven los pronombres de forma clara?
2. CONSISTENCIA: ¿Está libre de contradicciones fácticas internas o anacronismos?
3. RELEVANCIA Y PROFUNDIDAD: ¿Se desarrollan los temas en profundidad (mínimo 300 palabras por sección, sin viñetas vacías o párrafos sintéticos breves)?

DEVUELVE EL RESULTADO EN ESTE FORMATO DE CLAVES EXACTAS (una por línea):
COHESION_PASSED: true|false
CONSISTENCY_PASSED: true|false
RELEVANCE_DEPTH_PASSED: true|false
OVERALL_PASSED: true|false
FLAWED_SECTIONS: Lista de títulos de secciones con deficiencias (o "Ninguna")
INSTRUCTIONS: Instrucciones concretas para subsanar los fallos señalados (o "Ninguna")`)
	prompt := fmt.Sprintf(promptTemplate, topic, report)

	messages := []llm.Message{
		{Role: "system", Content: researchText(ctx,
			"You are an independent critical Deep Research evaluator based on Reinhart's discourse coherence theory.",
			"Eres un Evaluador Crítico e Independiente de Investigación en Deep Research basado en la Teoría de Coherencia Discursiva de Reinhart.",
		) + "\n\n" + reportLanguageDirective(ctx)},
		{Role: "user", Content: prompt},
	}

	resp, err := c.llm.GenerateCompletion(ctx, messages, 0.1)
	if err != nil {
		return nil, fmt.Errorf("evaluación crítica falló: %w", err)
	}

	return parseRubricResponse(resp), nil
}

func parseRubricResponse(resp string) *ReinhartRubricResult {
	res := &ReinhartRubricResult{
		CohesionPassed:       true,
		ConsistencyPassed:    true,
		RelevanceDepthPassed: true,
		OverallPassed:        true,
	}

	lines := strings.Split(resp, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)

		switch {
		case strings.HasPrefix(upper, "COHESION_PASSED:"):
			val := strings.TrimSpace(trimmed[len("COHESION_PASSED:"):])
			res.CohesionPassed = strings.EqualFold(val, "true")
		case strings.HasPrefix(upper, "CONSISTENCY_PASSED:"):
			val := strings.TrimSpace(trimmed[len("CONSISTENCY_PASSED:"):])
			res.ConsistencyPassed = strings.EqualFold(val, "true")
		case strings.HasPrefix(upper, "RELEVANCE_DEPTH_PASSED:"):
			val := strings.TrimSpace(trimmed[len("RELEVANCE_DEPTH_PASSED:"):])
			res.RelevanceDepthPassed = strings.EqualFold(val, "true")
		case strings.HasPrefix(upper, "OVERALL_PASSED:"):
			val := strings.TrimSpace(trimmed[len("OVERALL_PASSED:"):])
			res.OverallPassed = strings.EqualFold(val, "true")
		case strings.HasPrefix(upper, "FLAWED_SECTIONS:"):
			val := strings.TrimSpace(trimmed[len("FLAWED_SECTIONS:"):])
			if val != "" && !strings.EqualFold(val, "Ninguna") && !strings.EqualFold(val, "None") {
				sections := strings.Split(val, ",")
				for _, s := range sections {
					cleaned := strings.TrimSpace(s)
					if cleaned != "" {
						res.FlawedSections = append(res.FlawedSections, cleaned)
					}
				}
			}
		case strings.HasPrefix(upper, "INSTRUCTIONS:"):
			val := strings.TrimSpace(trimmed[len("INSTRUCTIONS:"):])
			res.Instructions = val
		}
	}

	if !res.CohesionPassed || !res.ConsistencyPassed || !res.RelevanceDepthPassed {
		res.OverallPassed = false
	}

	return res
}

// RefineReport performs localized rewrites on flawed sections of the report draft.
func (c *CriticLoop) RefineReport(ctx context.Context, topic string, report string, rubric *ReinhartRubricResult, sources []CitationSource) (string, error) {
	if c.llm == nil {
		return report, nil
	}

	promptTemplate := researchText(ctx, `Research topic: %q

CURRENT REPORT:
%s

CRITICAL EVALUATION AND IDENTIFIED ISSUES:
- Cohesion: %v
- Consistency: %v
- Relevance and Depth: %v
- Deficient sections: %s
- Correction instructions: %s

LOCALIZED REWRITE INSTRUCTIONS:
1. Rewrite only deficient sections, or apply the guidance to improve overall depth and cohesion.
2. Keep the rest of the report intact without changing valid data or arguments.
3. Strictly preserve numeric [n] citations within the valid reference-catalogue range ([1] to [%d]).
4. Return the complete corrected Markdown report without introductory or trailing explanations.`, `Tema de la investigación: %q

INFORME ACTUAL:
%s

EVALUACIÓN CRÍTICA Y FALLOS DETECTADOS:
- Cohesión: %v
- Consistencia: %v
- Relevancia y Profundidad: %v
- Secciones con deficiencias: %s
- Instrucciones de Corrección: %s

INSTRUCCIONES DE REESCRITURA LOCALIZADA:
1. Reescribe únicamente las secciones con deficiencias o aplícalas para mejorar la profundidad y cohesión general.
2. Mantén intacto el resto del informe sin alterar datos o argumentos válidos.
3. Conserva rigurosamente las citas numéricas [n] en el rango válido del catálogo de referencias ([1] a [%d]).
4. Devuelve el informe Markdown completo corregido sin agregar explicaciones previas o posteriores.`)
	prompt := fmt.Sprintf(promptTemplate, topic, report, rubric.CohesionPassed, rubric.ConsistencyPassed, rubric.RelevanceDepthPassed, strings.Join(rubric.FlawedSections, ", "), rubric.Instructions, len(sources))

	messages := []llm.Message{
		{Role: "system", Content: researchText(ctx,
			"You are Pharus, the lead technical Deep Research writer. Apply localized discourse and depth corrections without breaking citations or report coherence.",
			"Eres Pharus, redactor técnico principal de Deep Research. Aplicas correcciones discursivas y de profundidad de forma localizada sin romper citas ni perder la coherencia del informe.",
		) + "\n\n" + reportLanguageDirective(ctx)},
		{Role: "user", Content: prompt},
	}

	refined, err := c.llm.GenerateCompletion(ctx, messages, 0.2)
	if err != nil {
		return "", fmt.Errorf("refinamiento de informe falló: %w", err)
	}

	return refined, nil
}

// RunCriticLoop executes the bounded loop engineering refinement cycle with cosine similarity convergence stopping.
func (c *CriticLoop) RunCriticLoop(ctx context.Context, topic string, initialReport string, sources []CitationSource) (string, ResearchStatus, []string, error) {
	if c.llm == nil {
		return initialReport, ResearchStatusSuccess, nil, nil
	}

	log := logger.Get()
	currentReport := initialReport
	warnings := make([]string, 0)
	status := ResearchStatusSuccess

	var prevEmbedding []float32
	if c.embed != nil {
		if emb, err := c.embed.Embed(ctx, currentReport); err == nil {
			prevEmbedding = emb
		}
	}

	for iter := 1; iter <= c.config.MaxIterations; iter++ {
		log.Info("running critic loop iteration", "iteration", iter, "max_iterations", c.config.MaxIterations)

		rubric, err := c.EvaluateReport(ctx, topic, currentReport)
		if err != nil {
			log.Warn("critic loop evaluation warning", "error", err)
			warnings = append(warnings, fmt.Sprintf("Evaluación crítica incompleta en iteración %d: %v", iter, err))
			break
		}

		if rubric.OverallPassed {
			log.Info("critic loop passed all Reinhart criteria", "iteration", iter)
			break
		}

		refined, err := c.RefineReport(ctx, topic, currentReport, rubric, sources)
		if err != nil {
			log.Warn("critic loop refinement warning", "error", err)
			warnings = append(warnings, fmt.Sprintf("Refinamiento crítico falló en iteración %d: %v", iter, err))
			break
		}

		// Validate citations in refined report
		if len(sources) > 0 {
			if validationErr := validateSynthesizedReport(refined, sources); validationErr != nil {
				log.Warn("critic loop refinement produced an invalid report; attempting repair", "error_kind", observableErrorKind(validationErr))
				synth := NewSynthesizer(c.llm)
				repaired, repErr := synth.repairCitations(ctx, refined, sources, validationErr)
				if repErr == nil {
					refined = repaired
				} else {
					log.Warn("critic loop citation repair failed; reverting iteration refinement", "error", repErr)
					warnings = append(warnings, fmt.Sprintf("Refinamiento en iteración %d descartado por inconsistencia de citas.", iter))
					break
				}
			}
		}

		// Calculate embedding similarity for convergence stopping condition
		if c.embed != nil && len(prevEmbedding) > 0 {
			currEmbedding, embErr := c.embed.Embed(ctx, refined)
			if embErr == nil && len(currEmbedding) == len(prevEmbedding) {
				sim := float64(vectordb.CosineSimilarity(prevEmbedding, currEmbedding))
				log.Info("critic loop convergence calculated", "iteration", iter, "cosine_similarity", sim, "threshold", c.config.ConvergenceThreshold)

				if sim >= c.config.ConvergenceThreshold {
					log.Info("critic loop reached convergence threshold", "cosine_similarity", sim)
					warnings = append(warnings, fmt.Sprintf("Bucle de crítica alcanzó umbral de convergencia (similitud del coseno: %.4f >= %.2f).", sim, c.config.ConvergenceThreshold))
					currentReport = refined
					break
				}
				prevEmbedding = currEmbedding
			}
		}

		currentReport = refined

		if iter == c.config.MaxIterations {
			log.Info("critic loop reached max iterations", "max_iterations", c.config.MaxIterations)
			finalRubric, _ := c.EvaluateReport(ctx, topic, currentReport)
			if finalRubric != nil && !finalRubric.OverallPassed {
				status = ResearchStatusDegraded
				warnings = append(warnings, "El informe final fue entregado tras agotar las iteraciones del bucle de crítica con advertencias de profundidad o consistencia.")
			}
		}
	}

	return currentReport, status, warnings, nil
}
