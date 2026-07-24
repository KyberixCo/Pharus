package research

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KyberixCo/Pharus/internal/llm"
	"github.com/KyberixCo/Pharus/internal/scraper"
)

type countingProvider struct {
	calls int
}

func (p *countingProvider) GenerateCompletion(ctx context.Context, messages []llm.Message, temperature float64) (string, error) {
	p.calls++
	return "unexpected synthesis", nil
}

func TestExecuteResearchResultFailsClosedWhenSearchIsUnavailable(t *testing.T) {
	searchCalls := 0
	searx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searchCalls++
		http.Error(w, "JSON search is disabled", http.StatusForbidden)
	}))
	defer searx.Close()

	provider := &countingProvider{}
	engine := &Engine{
		searx:     scraper.NewSearXNGClient(searx.URL),
		discourse: NewDiscourseManager(nil),
		synth:     NewSynthesizer(provider),
		// dataSTORM, vectorDB and synth deliberately stay nil. Reaching any of
		// them would panic and prove that the workflow did not fail closed.
	}

	result, err := engine.ExecuteResearchResult(context.Background(), "protección laboral")
	if err == nil {
		t.Fatal("expected research to fail when every search request is rejected")
	}
	if result == nil {
		t.Fatal("expected a structured failed result")
	}
	if result.Status != ResearchStatusFailed {
		t.Fatalf("expected failed status, got %q", result.Status)
	}
	if result.FailureCode != FailureCodeSearchUnavailable {
		t.Fatalf("expected %q, got %q", FailureCodeSearchUnavailable, result.FailureCode)
	}
	if result.Report != "" {
		t.Fatalf("failed research must not contain a report, got %q", result.Report)
	}
	if result.EvidenceCount != 0 {
		t.Fatalf("expected zero evidence, got %d", result.EvidenceCount)
	}
	if searchCalls == 0 {
		t.Fatal("expected at least one search request")
	}
	if FailureCodeOf(err) != FailureCodeSearchUnavailable {
		t.Fatalf("unexpected error code: %q", FailureCodeOf(err))
	}
	if provider.calls != 0 {
		t.Fatalf("the synthesizer must not be called without evidence; got %d calls", provider.calls)
	}
}

func TestExecuteResearchResultFailsClosedWhenSearchHasNoUsableDocuments(t *testing.T) {
	searx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results": []}`))
	}))
	defer searx.Close()

	engine := &Engine{
		searx:     scraper.NewSearXNGClient(searx.URL),
		discourse: NewDiscourseManager(nil),
	}

	result, err := engine.ExecuteResearchResult(context.Background(), "tema sin resultados")
	if err == nil {
		t.Fatal("expected research to fail when search has no usable documents")
	}
	if result == nil || result.Status != ResearchStatusFailed {
		t.Fatalf("expected structured failed result, got %+v", result)
	}
	if result.FailureCode != FailureCodeEvidenceInsufficient {
		t.Fatalf("expected %q, got %q", FailureCodeEvidenceInsufficient, result.FailureCode)
	}
	if result.Report != "" {
		t.Fatal("failed research must not synthesize a report")
	}

	var researchErr *ResearchError
	if !errors.As(err, &researchErr) {
		t.Fatalf("expected ResearchError, got %T", err)
	}
}
