package research

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/KyberixCo/Pharus/internal/llm"
)

const (
	defaultQueryMaxCharacters = 160
	defaultQueryMaxTerms      = 20
	minPrimaryQueries         = 3
	maxPrimaryQueries         = 6
	maxGapQueries             = 2
)

// SearchQuery is a bounded search expression and its purpose. It is retained
// in the result and copied to every document it retrieves for provenance.
type SearchQuery struct {
	Text        string `json:"text"`
	Purpose     string `json:"purpose"`
	Perspective string `json:"perspective"`
	Priority    int    `json:"priority"`
}

const defaultQuerySimilarityThreshold = 0.82

type QueryPlannerConfig struct {
	MaxCharacters       int
	MaxTerms            int
	SimilarityThreshold float64
}

// QueryPlanner produces short, engine-friendly queries. Invalid LLM output is
// never used; planning falls back to a deterministic plan instead.
type QueryPlanner struct {
	llm llm.Provider
	cfg QueryPlannerConfig
}

func NewQueryPlanner(provider llm.Provider, cfg QueryPlannerConfig) *QueryPlanner {
	if cfg.MaxCharacters <= 0 {
		cfg.MaxCharacters = defaultQueryMaxCharacters
	}
	if cfg.MaxTerms <= 0 {
		cfg.MaxTerms = defaultQueryMaxTerms
	}
	if cfg.SimilarityThreshold <= 0 {
		cfg.SimilarityThreshold = defaultQuerySimilarityThreshold
	}
	return &QueryPlanner{llm: provider, cfg: cfg}
}

func (p *QueryPlanner) PlanPrimary(ctx context.Context, topic string, roles []ExpertRole) ([]SearchQuery, error) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return nil, fmt.Errorf("research topic is empty")
	}
	if p.llm != nil {
		if q, err := p.planWithLLM(ctx, topic, roles, false, ""); err == nil && len(q) >= minPrimaryQueries {
			return q, nil
		} else {
			loggerForResearchContext(ctx).Warn("research query planning fallback activated",
				"phase", "planning",
				"operation", "primary_query_plan",
				"fallback", "deterministic",
				"error_kind", observableErrorKind(err),
			)
		}
	}
	q := p.fallbackPrimary(topic, roles)
	if len(q) < minPrimaryQueries {
		return nil, fmt.Errorf("query planner could not produce %d valid queries", minPrimaryQueries)
	}
	return q, nil
}

// PlanGaps creates at most two queries after reviewing initial evidence.
func (p *QueryPlanner) PlanGaps(ctx context.Context, topic, evidenceSummary string) ([]SearchQuery, error) {
	if strings.TrimSpace(evidenceSummary) == "" {
		return nil, nil
	}
	if p.llm != nil {
		if q, err := p.planWithLLM(ctx, topic, nil, true, evidenceSummary); err == nil {
			return q, nil
		} else {
			loggerForResearchContext(ctx).Warn("research gap planning fallback activated",
				"phase", "planning",
				"operation", "evidence_gap_analysis",
				"fallback", "deterministic",
				"error_kind", observableErrorKind(err),
			)
		}
	}
	return p.normalizeQueries([]SearchQuery{{Text: topic + " limitations exceptions official sources", Purpose: "evidence gap: limitations and exceptions", Perspective: "gap analysis", Priority: 7}}, nil, maxGapQueries), nil
}

func (p *QueryPlanner) planWithLLM(ctx context.Context, topic string, roles []ExpertRole, gaps bool, evidenceSummary string) ([]SearchQuery, error) {
	roleText := make([]string, 0, len(roles))
	for _, role := range roles {
		desc := role.Name + ": " + role.Perspective
		if role.CognitiveBias != "" {
			desc += researchText(ctx, " [Bias: ", " [Sesgo: ") + role.CognitiveBias + "]"
		}
		roleText = append(roleText, desc)
	}
	count := "3 a 6"
	if gaps {
		count = "0 a 2"
	}
	promptTemplate := researchText(ctx,
		"Topic: %q\nCo-STORM expert perspectives and biases: %s\n\nGenerate %s orthogonal web queries. Use only keywords, proper names, jurisdictions, dates, and operators such as site:. Do not write questions or explanations. Return one query per line exactly as:\npurpose | perspective | query",
		"Tema: %q\nPerspectivas y Sesgos Experto Co-STORM: %s\n\nGenera %s consultas web ortogonales. Usa solamente palabras clave, nombres propios, jurisdicciones, fechas y operadores como site:. No redactes preguntas ni explicaciones. Devuelve una línea por consulta exactamente así:\npropósito | perspectiva | consulta",
	)
	prompt := fmt.Sprintf(promptTemplate, topic, strings.Join(roleText, "; "), count)
	if gaps {
		prompt += researchText(ctx,
			"\nEvidence summary (use it only to identify gaps; do not copy it):\n",
			"\nResumen de evidencia (úsalo solo para detectar vacíos, no lo copies):\n",
		) + evidenceSummary
	}
	systemPrompt := researchText(ctx,
		"You are a search-engine query planner. Respond only with structured lines of orthogonal keywords.",
		"Eres un planificador de consultas para motores de búsqueda. Respondes solo con líneas estructuradas de palabras clave ortogonales.",
	)
	resp, err := p.llm.GenerateCompletion(ctx, []llm.Message{{Role: "system", Content: systemPrompt + "\n\n" + reportLanguageDirective(ctx)}, {Role: "user", Content: prompt}}, 0.1)
	if err != nil {
		return nil, err
	}
	if err := ValidateReportLanguage(resp); err != nil {
		return nil, err
	}
	var candidate []SearchQuery
	for _, line := range strings.Split(resp, "\n") {
		parts := strings.SplitN(line, "|", 3)
		if len(parts) == 3 {
			candidate = append(candidate, SearchQuery{Purpose: strings.TrimSpace(parts[0]), Perspective: strings.TrimSpace(parts[1]), Text: strings.TrimSpace(parts[2])})
		}
	}
	limit := maxPrimaryQueries
	if gaps {
		limit = maxGapQueries
	}
	queries := p.normalizeQueries(candidate, roles, limit)
	if (!gaps && len(queries) < minPrimaryQueries) || (gaps && len(candidate) > 0 && len(queries) == 0) {
		return nil, fmt.Errorf("LLM returned an invalid query plan")
	}
	return queries, nil
}

func (p *QueryPlanner) fallbackPrimary(topic string, roles []ExpertRole) []SearchQuery {
	candidates := []SearchQuery{{Text: topic, Purpose: "topic overview", Perspective: "general", Priority: 1}, {Text: topic + " official sources", Purpose: "authoritative sources", Perspective: "official", Priority: 2}, {Text: topic + " law regulation case law", Purpose: "rules and decisions", Perspective: "legal", Priority: 3}}
	for _, group := range [][]string{{"lactancia", "fuero de maternidad", "maternity leave", "breastfeeding"}, {"madre cabeza de familia", "single mother", "head of household"}, {"trabajo remoto", "teletrabajo", "trabajo en casa", "remote work", "telework"}} {
		if keyword := firstContained(topic, group); keyword != "" {
			candidates = append(candidates, SearchQuery{Text: keyword + " " + jurisdictionTerms(topic) + " normativa oficial", Purpose: "specific legal protection", Perspective: keyword, Priority: len(candidates) + 1})
		}
	}
	for _, role := range roles {
		if keywords := perspectiveKeywords(role.Perspective); keywords != "" {
			candidates = append(candidates, SearchQuery{Text: topic + " " + keywords, Purpose: role.Name, Perspective: role.Perspective, Priority: len(candidates) + 1})
		}
	}
	return p.normalizeQueries(candidates, roles, maxPrimaryQueries)
}

func (p *QueryPlanner) normalizeQueries(candidates []SearchQuery, roles []ExpertRole, limit int) []SearchQuery {
	perspectives := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		perspectives[canonical(role.Perspective)] = struct{}{}
	}
	seen := make(map[string]struct{}, len(candidates))
	queries := make([]SearchQuery, 0, limit)
	for _, query := range candidates {
		query.Text = p.normalizeText(query.Text)
		key := canonical(query.Text)
		if query.Text == "" || len(strings.Fields(query.Text)) > p.cfg.MaxTerms || len([]rune(query.Text)) > p.cfg.MaxCharacters {
			continue
		}
		if _, narrative := perspectives[key]; narrative {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		// Enforce Query Orthogonality Filter
		if !isOrthogonal(query.Text, queries, p.cfg.SimilarityThreshold) {
			continue
		}
		seen[key] = struct{}{}
		if query.Purpose == "" {
			query.Purpose = "research"
		}
		if query.Perspective == "" {
			query.Perspective = "general"
		}
		if query.Priority <= 0 {
			query.Priority = len(queries) + 1
		}
		queries = append(queries, query)
		if len(queries) == limit {
			break
		}
	}
	sort.SliceStable(queries, func(i, j int) bool { return queries[i].Priority < queries[j].Priority })
	return queries
}

func isOrthogonal(candidate string, existing []SearchQuery, similarityThreshold float64) bool {
	candTerms := termSet(candidate)
	if len(candTerms) == 0 {
		return false
	}
	for _, q := range existing {
		exTerms := termSet(q.Text)
		if len(exTerms) == 0 {
			continue
		}
		sim := jaccardSimilarity(candTerms, exTerms)
		// Jaccard threshold: 0.75 filters near-identical query variants while retaining legitimate modifier queries
		jaccardThreshold := 0.75
		if sim >= jaccardThreshold {
			return false
		}
	}
	return true
}

func termSet(text string) map[string]struct{} {
	words := strings.Fields(strings.ToLower(text))
	set := make(map[string]struct{}, len(words))
	for _, w := range words {
		w = strings.Trim(w, " \t\n\r,;.!?\"'()")
		if len(w) > 2 {
			set[w] = struct{}{}
		}
	}
	return set
}

func jaccardSimilarity(setA, setB map[string]struct{}) float64 {
	intersection := 0
	for k := range setA {
		if _, exists := setB[k]; exists {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union <= 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

var queryPrefix = regexp.MustCompile(`(?i)^\s*(?:[-*•]\s*|\d+[.)]\s*)?(?:consulta(?: de búsqueda)?|search query|query)\s*:\s*`)

func (p *QueryPlanner) normalizeText(text string) string {
	text = queryPrefix.ReplaceAllString(strings.TrimSpace(text), "")
	return strings.Trim(strings.Join(strings.Fields(text), " "), " \t\n\r,;.!?")
}

func perspectiveKeywords(text string) string {
	words := strings.Fields(text)
	if len(words) > 6 {
		words = words[:6]
	}
	return strings.Join(words, " ")
}
func jurisdictionTerms(topic string) string {
	for _, term := range []string{"Colombia", "colombiana", "Colombian", "España", "Mexico", "México"} {
		if strings.Contains(strings.ToLower(topic), strings.ToLower(term)) {
			return term
		}
	}
	return ""
}
func firstContained(text string, candidates []string) string {
	lower := strings.ToLower(text)
	for _, candidate := range candidates {
		if strings.Contains(lower, strings.ToLower(candidate)) {
			return candidate
		}
	}
	return ""
}
func canonical(text string) string { return strings.ToLower(strings.Join(strings.Fields(text), " ")) }
