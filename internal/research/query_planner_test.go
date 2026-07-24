package research

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/KyberixCo/Pharus/internal/llm"
)

type plannerProvider struct {
	response string
	err      error
}

func (p plannerProvider) GenerateCompletion(context.Context, []llm.Message, float64) (string, error) {
	return p.response, p.err
}

func TestQueryPlannerFallbackProducesSeparateColombianEmploymentQueries(t *testing.T) {
	planner := NewQueryPlanner(nil, QueryPlannerConfig{})
	topic := "protección laboral en Colombia: lactancia, madre cabeza de familia y teletrabajo"
	queries, err := planner.PlanPrimary(context.Background(), topic, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) < 3 {
		t.Fatalf("expected at least 3 queries, got %d", len(queries))
	}
	joined := strings.ToLower(joinQueryText(queries))
	for _, term := range []string{"lactancia", "madre cabeza de familia", "teletrabajo"} {
		if !strings.Contains(joined, term) {
			t.Errorf("missing separate subject %q in %q", term, joined)
		}
	}
	if !strings.Contains(joined, "colombia") || !strings.Contains(joined, "oficial") {
		t.Errorf("expected Colombian official-source query, got %q", joined)
	}
	assertBoundedQueries(t, queries)
}

func TestQueryPlannerLLMInvalidOutputFallsBack(t *testing.T) {
	planner := NewQueryPlanner(plannerProvider{response: "Esta es una explicación extensa, no un plan."}, QueryPlannerConfig{})
	queries, err := planner.PlanPrimary(context.Background(), "Ley 123 de 2024 site:gov.co", []ExpertRole{{Perspective: "Una narrativa explicativa que no debe ser consulta"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) < 3 {
		t.Fatalf("expected fallback plan, got %#v", queries)
	}
	assertBoundedQueries(t, queries)
}

func TestQueryPlannerNormalizesAndRejectsNarrativesAndDuplicates(t *testing.T) {
	roles := []ExpertRole{{Perspective: "Evaluación narrativa completa de riesgos y cumplimiento"}}
	provider := plannerProvider{response: "" +
		"base | general | Consulta: Ley García Márquez 2024 site:gov.co.\n" +
		"duplicate | general | ley garcía márquez 2024 site:gov.co\n" +
		"narrative | general | Evaluación narrativa completa de riesgos y cumplimiento\n" +
		"remote | work | remote work regulation 2024 site:example.org\n" +
		"other | legal | employment rights 2024 site:gov.co"}
	planner := NewQueryPlanner(provider, QueryPlannerConfig{})
	queries, err := planner.PlanPrimary(context.Background(), "employment law", roles)
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 3 {
		t.Fatalf("expected exactly three valid LLM queries, got %#v", queries)
	}
	if queries[0].Text != "Ley García Márquez 2024 site:gov.co" {
		t.Errorf("normalization lost proper name/operator: %q", queries[0].Text)
	}
	assertBoundedQueries(t, queries)
}

func TestQueryPlannerFallsBackWhenLLMFails(t *testing.T) {
	planner := NewQueryPlanner(plannerProvider{err: errors.New("unavailable")}, QueryPlannerConfig{MaxCharacters: 160, MaxTerms: 20})
	queries, err := planner.PlanPrimary(context.Background(), "2024 Data Protection Act site:gov.uk", nil)
	if err != nil {
		t.Fatal(err)
	}
	assertBoundedQueries(t, queries)
}

func TestQueryPlannerGapLimit(t *testing.T) {
	planner := NewQueryPlanner(plannerProvider{response: "one | gap | law exceptions site:gov.co\ntwo | gap | enforcement decisions site:gov.co\nthree | gap | unrelated third query"}, QueryPlannerConfig{})
	queries, err := planner.PlanGaps(context.Background(), "employment law", "some evidence")
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 2 {
		t.Fatalf("expected two gap queries, got %#v", queries)
	}
}

func TestQueryPlannerOrthogonalityFilter(t *testing.T) {
	provider := plannerProvider{response: "" +
		"base | general | mcp architecture design standards 2024\n" +
		"similar | general | mcp architecture design standards 2024 site:gov.co\n" +
		"orthogonal1 | security | mcp security risks vulnerabilities\n" +
		"orthogonal2 | performance | mcp latency throughput benchmark"}
	planner := NewQueryPlanner(provider, QueryPlannerConfig{})
	queries, err := planner.PlanPrimary(context.Background(), "mcp architecture", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range queries {
		if strings.Contains(q.Text, "site:gov.co") {
			t.Errorf("expected redundant non-orthogonal query to be filtered out, got: %q", q.Text)
		}
	}
}

func assertBoundedQueries(t *testing.T, queries []SearchQuery) {
	t.Helper()
	seen := map[string]bool{}
	for _, q := range queries {
		if len([]rune(q.Text)) > 160 || len(strings.Fields(q.Text)) > 20 {
			t.Errorf("query exceeds limit: %q", q.Text)
		}
		key := strings.ToLower(q.Text)
		if seen[key] {
			t.Errorf("duplicate query: %q", q.Text)
		}
		seen[key] = true
	}
}

func joinQueryText(queries []SearchQuery) string {
	parts := make([]string, 0, len(queries))
	for _, q := range queries {
		parts = append(parts, q.Text)
	}
	return strings.Join(parts, "\n")
}
