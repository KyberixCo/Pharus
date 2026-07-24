package benchmark

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/KyberixCo/Pharus/internal/llm"
	"github.com/KyberixCo/Pharus/internal/research"
)

type benchmarkEngineStub struct {
	report string
	err    error
}

func (s benchmarkEngineStub) ExecuteResearch(context.Context, string) (string, error) {
	return s.report, s.err
}

type benchmarkJudgeStub struct {
	response string
	err      error
}

func (s benchmarkJudgeStub) GenerateCompletion(context.Context, []llm.Message, float64) (string, error) {
	return s.response, s.err
}

func TestDatasetLoader_GetBuiltInSuite(t *testing.T) {
	loader := NewDatasetLoader()

	suites := []string{
		SuiteDeepResearchBench,
		SuiteBrowseComp,
		SuiteGAIA,
		SuiteHLE,
		SuiteSynthetic,
	}

	for _, suite := range suites {
		items, err := loader.GetBuiltInSuite(suite)
		if err != nil {
			t.Fatalf("unexpected error getting suite %s: %v", suite, err)
		}
		if len(items) == 0 {
			t.Errorf("expected non-empty items for suite %s", suite)
		}
	}
}

func TestDatasetLoader_JSONL(t *testing.T) {
	tmpDir := t.TempDir()
	jsonlPath := filepath.Join(tmpDir, "test_dataset.jsonl")

	content := `{"id":"test-1","suite":"deep_research_bench","topic":"Topic 1"}
{"id":"test-2","suite":"deep_research_bench","topic":"Topic 2"}
`
	if err := os.WriteFile(jsonlPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	loader := NewDatasetLoader()
	items, err := loader.LoadFromFile(jsonlPath)
	if err != nil {
		t.Fatalf("failed to load jsonl dataset: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].ID != "test-1" || items[1].ID != "test-2" {
		t.Errorf("unexpected item contents: %+v", items)
	}
}

func TestBenchmarkRunner_RunSuite(t *testing.T) {
	loader := NewDatasetLoader()
	items, err := loader.GetBuiltInSuite(SuiteSynthetic)
	if err != nil {
		t.Fatalf("failed to get synthetic suite: %v", err)
	}

	evaluator := research.NewRACEEvaluator(benchmarkJudgeStub{response: "Relevance: 0.8\nAuthenticity: 0.7\nClarity: 0.9\nEvidence: 0.6"})
	runner := NewBenchmarkRunner(benchmarkEngineStub{report: "# Informe\n\n[1]"}, evaluator)
	opts := RunOptions{Limit: 2, PassCutoff: 0.70}

	summary, err := runner.RunSuite(context.Background(), items, opts)
	if err != nil {
		t.Fatalf("RunSuite failed: %v", err)
	}

	if len(summary.Results) != 2 {
		t.Errorf("expected 2 results due to limit, got %d", len(summary.Results))
	}

	if summary.Overall.AvgOverallRACE == 0 {
		t.Errorf("expected non-zero RACE score overall")
	}

	tmpDir := t.TempDir()
	reporter := NewBenchmarkReporter()
	jsonPath := filepath.Join(tmpDir, "report.json")
	mdPath := filepath.Join(tmpDir, "report.md")

	if err := reporter.ExportJSON(summary, jsonPath); err != nil {
		t.Errorf("ExportJSON failed: %v", err)
	}

	if err := reporter.ExportMarkdown(summary, mdPath); err != nil {
		t.Errorf("ExportMarkdown failed: %v", err)
	}

	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		t.Errorf("expected json report file to exist")
	}
	if _, err := os.Stat(mdPath); os.IsNotExist(err) {
		t.Errorf("expected md report file to exist")
	}
}

func TestBenchmarkRunner_RequiresRealExecutionAndEvaluation(t *testing.T) {
	items := []DatasetItem{{ID: "item", Suite: SuiteSynthetic, Topic: "tema"}}
	if _, err := NewBenchmarkRunner(nil, nil).RunSuite(context.Background(), items, RunOptions{}); err == nil {
		t.Fatal("expected runner without dependencies to fail")
	}

	evaluator := research.NewRACEEvaluator(benchmarkJudgeStub{err: errors.New("judge unavailable")})
	runner := NewBenchmarkRunner(benchmarkEngineStub{report: "# Informe"}, evaluator)
	summary, err := runner.RunSuite(context.Background(), items, RunOptions{})
	if err != nil {
		t.Fatalf("RunSuite returned unexpected setup error: %v", err)
	}
	if summary.Results[0].Success || summary.Results[0].Score != nil || summary.Results[0].Error == "" {
		t.Fatalf("judge failure must be recorded rather than scored: %+v", summary.Results[0])
	}
}
