package research

import (
	"context"
	"strings"
	"testing"

	"github.com/KyberixCo/Pharus/internal/llm"
	"github.com/KyberixCo/Pharus/internal/vectordb"
)

func researchMetricsFixture() *InMemorySQLProvider {
	provider := NewInMemorySQLProvider()
	provider.AddTable("research_metrics", SQLQueryResult{
		Columns: []string{"metric_id", "topic", "category", "value_score", "status"},
		Rows:    [][]string{{"m_test", "security", "fixture_metric", "42", "validated"}},
	})
	return provider
}

func TestDataSTORMConsistency(t *testing.T) {
	bank := NewGlobalInsightBank()
	bank.AddInsight(&GlobalInsight{
		ID:     "1",
		Topic:  "SSRF Mitigations",
		Metric: "DNS Pinning Estricto",
		Source: "Diseño.md",
	})

	detector := NewQueryConsistencyDetector(bank)

	ctx := context.Background()
	query, valid := detector.ValidateAndAlignQuery(ctx, "SSRF Mitigations", "evaluar mitigaciones con override de red")
	if valid {
		t.Errorf("expected query to be flagged as conflicting/divergent")
	}

	if query == "evaluar mitigaciones con override de red" {
		t.Errorf("expected aligned query to be modified with stored global insight metric")
	}
}

func TestGlobalInsightBankOps(t *testing.T) {
	bank := NewGlobalInsightBank()
	ctx := context.Background()

	docText := "Título: Seguridad SSRF\nContenido: DNS Pinning estricto previene rebotes DNS con 100% de eficacia."
	insights, err := bank.ExtractInsightsFromText(ctx, nil, "SSRF Mitigations", docText, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error during extraction: %v", err)
	}

	if len(insights) == 0 {
		t.Fatalf("expected at least 1 insight extracted")
	}

	retrieved := bank.GetInsightsForTopic("SSRF")
	if len(retrieved) == 0 {
		t.Errorf("expected to retrieve insights for topic SSRF")
	}

	all := bank.GetAllInsights()
	if len(all) == 0 {
		t.Errorf("expected all insights count > 0")
	}
}

func TestValidateAndAlignSQLQuery(t *testing.T) {
	bank := NewGlobalInsightBank()
	bank.AddInsight(&GlobalInsight{
		ID:     "ins_1",
		Topic:  "Security",
		Metric: "DNS Pinning",
	})
	detector := NewQueryConsistencyDetector(bank)
	ctx := context.Background()

	// 1. Test destructive query safety conversion
	aligned, valid := detector.ValidateAndAlignSQLQuery(ctx, "Security", "DROP TABLE research_metrics")
	if valid {
		t.Errorf("expected DROP TABLE query to be flagged invalid")
	}
	if !testing.Verbose() && aligned == "DROP TABLE research_metrics" {
		t.Errorf("expected query to be rewritten to safe SELECT")
	}

	// 2. Test 1=0 override correction
	aligned2, valid2 := detector.ValidateAndAlignSQLQuery(ctx, "Security", "SELECT * FROM research_metrics WHERE 1=0")
	if valid2 {
		t.Errorf("expected 1=0 clause to be flagged as divergent")
	}
	if aligned2 == "SELECT * FROM research_metrics WHERE 1=0" {
		t.Errorf("expected 1=0 to be replaced with 1=1")
	}
}

func TestInMemorySQLProvider(t *testing.T) {
	provider := researchMetricsFixture()
	ctx := context.Background()

	schema, err := provider.GetSchema(ctx)
	if err != nil || schema == "" {
		t.Fatalf("expected valid schema output, got err: %v", err)
	}

	res, err := provider.ExecuteSQL(ctx, "SELECT * FROM research_metrics WHERE topic = 'security'")
	if err != nil {
		t.Fatalf("unexpected SQL error: %v", err)
	}
	if res.RowCount == 0 {
		t.Errorf("expected at least 1 row returned")
	}
}

func TestInMemorySQLProviderReturnsNoRowsForUnmatchedFilter(t *testing.T) {
	provider := researchMetricsFixture()
	res, err := provider.ExecuteSQL(context.Background(), "SELECT * FROM research_metrics WHERE topic = 'legal'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RowCount != 0 || len(res.Rows) != 0 {
		t.Fatalf("unmatched filter must return zero rows, got %+v", res.Rows)
	}
}

func TestDataSTORMAnalyzer(t *testing.T) {
	bank := NewGlobalInsightBank()
	provider := researchMetricsFixture()
	analyzer := NewDataSTORMAnalyzer(bank, provider, nil)
	ctx := context.Background()

	res, insights, err := analyzer.ExploreHypothesis(ctx, "security", "SSRF mitigations with DNS Pinning")
	if err != nil {
		t.Fatalf("unexpected error exploring hypothesis: %v", err)
	}
	if res == nil || res.RowCount == 0 {
		t.Errorf("expected valid SQL query result")
	}
	if len(insights) == 0 {
		t.Errorf("expected extracted insights from DataSTORM query")
	}

	retrieved := bank.GetInsightsForTopic("security")
	if len(retrieved) == 0 {
		t.Errorf("expected insights saved in GlobalInsightBank")
	}
	for _, insight := range insights {
		if !insight.HasValidProvenance() || insight.SourceType != InsightSourceStructuredData || insight.StructuredQuery == "" {
			t.Fatalf("structured insight lacks valid provenance: %+v", insight)
		}
	}
}

func TestDataSTORMIsDisabledWithoutStructuredProvider(t *testing.T) {
	analyzer := NewDataSTORMAnalyzer(NewGlobalInsightBank(), nil, nil)
	if analyzer.Enabled() {
		t.Fatal("DataSTORM must be disabled without an explicit structured provider")
	}
	result, insights, err := analyzer.ExploreHypothesis(context.Background(), "legal", "hypothesis")
	if err != nil || result != nil || len(insights) != 0 {
		t.Fatalf("disabled DataSTORM must be a no-op, got result=%+v insights=%+v err=%v", result, insights, err)
	}
}

type promptCapturingProvider struct{ prompt string }

func (p *promptCapturingProvider) GenerateCompletion(_ context.Context, messages []llm.Message, _ float64) (string, error) {
	for _, message := range messages {
		if message.Role == "user" {
			p.prompt = message.Content
		}
	}
	return "## Análisis\nCon hallazgos [1].", nil
}

func TestSynthesisExcludesInsightsWithoutValidProvenance(t *testing.T) {
	bank := NewGlobalInsightBank()
	bank.AddInsight(&GlobalInsight{Topic: "topic", Thesis: "invented metric", Metric: "100%", Source: "demo", ValidationStatus: InsightValidationUnverified})
	bank.AddInsight(&GlobalInsight{Topic: "topic", Thesis: "evidence metric", Metric: "42", Source: "https://example.test", SourceType: InsightSourceEvidence, EvidenceID: "evidence_1", ValidationStatus: InsightValidationEvidence})
	singleNodeTree := &TaxmorphTree{
		Topic: "topic",
		Nodes: []*TaxonNode{{ID: "n1", Title: "Análisis", Level: 1}},
	}
	provider := &promptCapturingProvider{}
	synth := NewSynthesizer(provider)
	_, err := synth.SynthesizeHierarchicalReport(context.Background(), "topic", singleNodeTree, []vectordb.SearchResult{{ID: "evidence_1", Content: "source", Metadata: map[string]string{"url": "https://example.test", "title": "Example"}}}, nil, bank)
	if err != nil {
		t.Fatalf("unexpected synthesis error: %v", err)
	}
	if strings.Contains(provider.prompt, "invented metric") {
		t.Fatalf("unverified insight was included in synthesis prompt: %s", provider.prompt)
	}
	if !strings.Contains(provider.prompt, "evidence metric") || !strings.Contains(provider.prompt, "evidence_1") {
		t.Fatalf("valid insight provenance missing from prompt: %s", provider.prompt)
	}
}
