package research

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/KyberixCo/Pharus/internal/llm"
)

type TargetAudience string

const (
	AudienceTechnical TargetAudience = "Technical"
	AudienceExecutive TargetAudience = "Executive"
	AudienceAcademic  TargetAudience = "Academic"
	AudienceGeneral   TargetAudience = "General"
)

type DepthLevel string

const (
	DepthOverview   DepthLevel = "Overview"
	DepthDeepDive   DepthLevel = "DeepDive"
	DepthExhaustive DepthLevel = "Exhaustive"
)

// TopicNodeSpec represents a structured section or node in the research taxonomy.
type TopicNodeSpec struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	KeyQuestions []string `json:"key_questions"`
	SubTopics    []string `json:"sub_topics,omitempty"`
}

// ResearchPlan is the comprehensive blueprint generated during Phase 1 pre-research.
type ResearchPlan struct {
	Topic           string          `json:"topic"`
	Audience        TargetAudience  `json:"audience"`
	Depth           DepthLevel      `json:"depth"`
	CoreHypothesis  string          `json:"core_hypothesis"`
	RequiredAspects []string        `json:"required_aspects"`
	Outline         []TopicNodeSpec `json:"outline"`
}

// ResearchIntent captures explicit user input or preferences for topic scope.
type ResearchIntent struct {
	Audience       TargetAudience `json:"audience,omitempty"`
	Depth          DepthLevel     `json:"depth,omitempty"`
	FocusAreas     []string       `json:"focus_areas,omitempty"`
	ExcludedTopics []string       `json:"excluded_topics,omitempty"`
}

type PlanBuilder struct {
	llm llm.Provider
}

func NewPlanBuilder(provider llm.Provider) *PlanBuilder {
	return &PlanBuilder{llm: provider}
}

// BuildPlan constructs a structured ResearchPlan given a topic and optional user intent.
func (pb *PlanBuilder) BuildPlan(ctx context.Context, topic string, intent *ResearchIntent) (*ResearchPlan, error) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return nil, fmt.Errorf("research topic is required")
	}

	audience := AudienceTechnical
	depth := DepthExhaustive
	var focusAreas []string
	var excludedTopics []string

	if intent != nil {
		if intent.Audience != "" {
			audience = intent.Audience
		}
		if intent.Depth != "" {
			depth = intent.Depth
		}
		focusAreas = intent.FocusAreas
		excludedTopics = intent.ExcludedTopics
	}

	if pb.llm != nil {
		plan, err := pb.buildWithLLM(ctx, topic, audience, depth, focusAreas, excludedTopics)
		if err == nil && plan != nil && plan.Validate() == nil {
			return plan, nil
		}
		loggerForResearchContext(ctx).Warn("research planning fallback activated",
			"phase", "planning",
			"operation", "research_plan",
			"fallback", "deterministic",
			"error_kind", observableErrorKind(err),
		)
	}

	return pb.fallbackPlan(ctx, topic, audience, depth, focusAreas, excludedTopics), nil
}

func (pb *PlanBuilder) buildWithLLM(ctx context.Context, topic string, audience TargetAudience, depth DepthLevel, focusAreas, excludedTopics []string) (*ResearchPlan, error) {
	promptTemplate := researchText(ctx, `Research Topic: %q
Target Audience: %s
Desired Depth: %s
Priority Focus Areas: %s
Excluded Topics: %s

Design a detailed and exhaustive deep-research plan (ResearchPlan) as strict JSON.
The JSON object MUST use this exact structure:
{
  "topic": %q,
  "audience": %q,
  "depth": %q,
  "core_hypothesis": "Central research hypothesis or question",
  "required_aspects": ["Key aspect 1", "Key aspect 2", "Key aspect 3"],
  "outline": [
    {
      "id": "section_1",
      "title": "1. Section Title",
      "description": "Analytical description of what this section will investigate",
      "key_questions": ["Research question 1?", "Question 2?"],
      "sub_topics": ["Subtopic A", "Subtopic B"]
    }
  ]
}

GENERATION RULES:
1. Respond ONLY with the valid JSON object. Do not add introductions, text outside JSON, or Markdown fences.
2. Include 4 to 6 detailed outline sections to ensure extensive and rigorous analytical development.
3. Each section must contain at least 2 specific key questions and 2 subtopics.`, `Tema de Investigación: %q
Público Objetivo: %s
Nivel de Profundidad Deseado: %s
Áreas de Enfoque Prioritarias: %s
Temas Excluidos: %s

Diseña un plan de investigación profunda (ResearchPlan) detallado y exhaustivo en formato JSON estricto.
El objeto JSON DEBE tener esta estructura exacta:
{
  "topic": %q,
  "audience": %q,
  "depth": %q,
  "core_hypothesis": "Hipótesis o pregunta central de investigación",
  "required_aspects": ["Aspecto clave 1", "Aspecto clave 2", "Aspecto clave 3"],
  "outline": [
    {
      "id": "seccion_1",
      "title": "1. Título de la Sección",
      "description": "Descripción analítica de lo que se investigará en esta sección",
      "key_questions": ["Pregunta de investigación 1?", "Pregunta 2?"],
      "sub_topics": ["Subtema A", "Subtema B"]
    }
  ]
}

REGLAS DE GENERACIÓN:
1. Responde ÚNICAMENTE con el objeto JSON válido. No agregues introducciones, texto fuera de JSON ni bloques markdown.
2. Incluye entre 4 y 6 secciones detalladas en 'outline' para garantizar un desarrollo analítico extenso y riguroso.
3. Cada sección debe tener al menos 2 preguntas clave específicas y 2 subtemas.`)
	prompt := fmt.Sprintf(promptTemplate, topic, audience, depth, strings.Join(focusAreas, ", "), strings.Join(excludedTopics, ", "), topic, audience, depth)

	messages := []llm.Message{
		{Role: "system", Content: researchText(ctx,
			"You are Pharus's Scientific and Technical Research Planner. You generate maximally detailed research plans with taxonomic decomposition as strict JSON.",
			"Eres un Planificador de Investigación Científica y Técnica de Pharus. Generas planes de investigación estructurados en JSON estricto con máximo detalle y descomposición taxonómica.",
		) + "\n\n" + reportLanguageDirective(ctx)},
		{Role: "user", Content: prompt},
	}

	resp, err := pb.llm.GenerateCompletion(ctx, messages, 0.2)
	if err != nil {
		return nil, fmt.Errorf("llm plan generation: %w", err)
	}

	cleanJSON := extractJSON(resp)
	var plan ResearchPlan
	if err := json.Unmarshal([]byte(cleanJSON), &plan); err != nil {
		return nil, fmt.Errorf("unmarshal research plan: %w", err)
	}

	if plan.Topic == "" {
		plan.Topic = topic
	}
	if plan.Audience == "" {
		plan.Audience = audience
	}
	if plan.Depth == "" {
		plan.Depth = depth
	}
	if err := validateResearchPlanLanguage(&plan); err != nil {
		return nil, err
	}

	return &plan, nil
}

func validateResearchPlanLanguage(plan *ResearchPlan) error {
	if plan == nil {
		return nil
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	return ValidateReportLanguage(string(data))
}

func (pb *PlanBuilder) fallbackPlan(ctx context.Context, topic string, audience TargetAudience, depth DepthLevel, focusAreas, excludedTopics []string) *ResearchPlan {
	if researchText(ctx, "en", "es") == "en" {
		reqAspects := []string{
			"Conceptual framework and state of the art",
			"Methodological evaluation and comparative analysis",
			"Practical use cases and best practices",
			"Challenges, risks, and future outlook",
		}
		reqAspects = append(reqAspects, focusAreas...)
		return &ResearchPlan{
			Topic:           topic,
			Audience:        audience,
			Depth:           depth,
			CoreHypothesis:  fmt.Sprintf("In-depth, multidimensional analysis and evaluation of %s.", topic),
			RequiredAspects: reqAspects,
			Outline: []TopicNodeSpec{
				{ID: "sec_executive_summary", Title: "1. Executive Summary and Main Thesis", Description: fmt.Sprintf("High-level synthesis and central analytical proposition about %s.", topic), KeyQuestions: []string{fmt.Sprintf("What is the current state of %s and its main implications?", topic), "Which empirical or theoretical findings are decisive?"}, SubTopics: []string{"Broad context", "Central analytical thesis", "Scope and boundaries"}},
				{ID: "sec_architecture", Title: "2. Architecture and Theoretical Foundations", Description: fmt.Sprintf("Detailed examination of the underlying components and principles of %s.", topic), KeyQuestions: []string{fmt.Sprintf("What are the key components and structural patterns of %s?", topic), "Which mechanisms ensure its operation, reliability, or efficiency?"}, SubTopics: []string{"Core principles", "Internal mechanisms and flows", "Critical dependencies"}},
				{ID: "sec_comparison", Title: "3. Methodological and Technical Comparison", Description: fmt.Sprintf("Qualitative and quantitative analysis of trade-offs, performance, and benchmarks for %s.", topic), KeyQuestions: []string{fmt.Sprintf("How does %s compare with alternatives in the market or literature?", topic), "Which empirical evaluation metrics matter?"}, SubTopics: []string{"Comparison criteria", "Technical and operational trade-offs", "Empirical evidence"}},
				{ID: "sec_practices", Title: "4. Practical Recommendations and Implementation", Description: fmt.Sprintf("Operational guidance and application patterns for %s.", topic), KeyQuestions: []string{fmt.Sprintf("What proven best practices apply when implementing %s?", topic), "Which common pitfalls or antipatterns should be avoided?"}, SubTopics: []string{"Implementation guidance", "Antipatterns and pitfalls", "Continuous optimization"}},
				{ID: "sec_future", Title: "5. Conclusions and Future Trends", Description: fmt.Sprintf("Evolution outlook, edge cases, and forward-looking view of %s.", topic), KeyQuestions: []string{fmt.Sprintf("Where is the %s ecosystem heading?", topic), "Which secondary research areas remain open?"}, SubTopics: []string{"Findings synthesis", "Risks and gaps", "Roadmap and trends"}},
			},
		}
	}
	reqAspects := []string{
		"Marco conceptual y estado del arte",
		"Evaluación metodológica y análisis comparativo",
		"Casos de uso prácticos y mejores prácticas",
		"Desafíos, riesgos y perspectivas futuras",
	}
	if len(focusAreas) > 0 {
		reqAspects = append(reqAspects, focusAreas...)
	}

	outline := []TopicNodeSpec{
		{
			ID:          "sec_resumen_tesis",
			Title:       "1. Resumen Ejecutivo y Tesis Principal",
			Description: fmt.Sprintf("Síntesis macro y propuesta analítica central sobre %s.", topic),
			KeyQuestions: []string{
				fmt.Sprintf("¿Cuál es el estado actual de %s y sus implicaciones principales?", topic),
				"¿Cuáles son los hallazgos empíricos o teóricos determinantes?",
			},
			SubTopics: []string{"Contexto macro", "Tesis analítica central", "Alcance y delimitación"},
		},
		{
			ID:          "sec_arquitectura_fundamentos",
			Title:       "2. Análisis de Arquitectura y Fundamentos Teóricos",
			Description: fmt.Sprintf("Examen detallado de los componentes subyacentes y principios de %s.", topic),
			KeyQuestions: []string{
				fmt.Sprintf("¿Cuáles son los componentes clave y patrones estructurales de %s?", topic),
				"¿Qué mecanismos garantizan su funcionamiento, confiabilidad o eficiencia?",
			},
			SubTopics: []string{"Principios fundamentales", "Mecanismos y flujos internos", "Dependencias críticas"},
		},
		{
			ID:          "sec_metodologia_comparativa",
			Title:       "3. Evaluación Metodológica y Comparativa Técnica",
			Description: fmt.Sprintf("Análisis cualitativo y cuantitativo de trade-offs, rendimiento y benchmarks de %s.", topic),
			KeyQuestions: []string{
				fmt.Sprintf("¿Cómo se compara %s frente a las alternativas existentes en el mercado o literatura?", topic),
				"¿Cuáles son las métricas de evaluación empírica relevantes?",
			},
			SubTopics: []string{"Criterios de comparación", "Trade-offs técnicos y operativos", "Evidencia empírica"},
		},
		{
			ID:          "sec_mejores_practicas",
			Title:       "4. Recomendaciones Prácticas e Implementación",
			Description: fmt.Sprintf("Directrices operativas y patrones de aplicación para %s.", topic),
			KeyQuestions: []string{
				fmt.Sprintf("¿Cuáles son las mejores prácticas comprobadas al implementar %s?", topic),
				"¿Qué trampas comunes o antipatrones deben evitarse?",
			},
			SubTopics: []string{"Guías de implementación", "Antipatrones y trampas", "Optimización continua"},
		},
		{
			ID:          "sec_conclusiones_futuro",
			Title:       "5. Conclusiones y Análisis de Tendencias Futuras",
			Description: fmt.Sprintf("Perspectivas de evolución, casos de borde y visión prospectiva de %s.", topic),
			KeyQuestions: []string{
				fmt.Sprintf("¿Hacia dónde evoluciona el ecosistema de %s?", topic),
				"¿Qué áreas de investigación secundaria permanecen abiertas?",
			},
			SubTopics: []string{"Síntesis de hallazgos", "Riesgos y vacíos", "Hoja de ruta y tendencias"},
		},
	}

	return &ResearchPlan{
		Topic:           topic,
		Audience:        audience,
		Depth:           depth,
		CoreHypothesis:  fmt.Sprintf("Análisis de alta profundidad y evaluación multidimensional de %s.", topic),
		RequiredAspects: reqAspects,
		Outline:         outline,
	}
}

func (p *ResearchPlan) ToJSON() (string, error) {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", fmt.Errorf("serialize research plan: %w", err)
	}
	return string(b), nil
}

func (p *ResearchPlan) Validate() error {
	if strings.TrimSpace(p.Topic) == "" {
		return fmt.Errorf("research plan topic is required")
	}
	if len(p.Outline) == 0 {
		return fmt.Errorf("research plan outline cannot be empty")
	}
	for i, node := range p.Outline {
		if strings.TrimSpace(node.Title) == "" {
			return fmt.Errorf("outline node %d title is required", i)
		}
	}
	return nil
}

func extractJSON(input string) string {
	start := strings.Index(input, "{")
	end := strings.LastIndex(input, "}")
	if start != -1 && end != -1 && end > start {
		return input[start : end+1]
	}
	return strings.TrimSpace(input)
}
