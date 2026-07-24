package research

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KyberixCo/Pharus/internal/scraper"
)

type evidenceFetcher struct {
	pages map[string][]byte
	err   error
}

func (f evidenceFetcher) Fetch(_ context.Context, rawURL string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.pages[rawURL], nil
}

func TestEvidenceCollectorDeduplicatesCanonicalURLAndPreservesQueries(t *testing.T) {
	page := []byte("<html><title>Fuente oficial</title><body>Este es contenido completo suficiente para una evidencia trazable de investigación y pruebas unitarias.</body></html>")
	collector := NewEvidenceCollector(evidenceFetcher{pages: map[string][]byte{"https://example.com/r?a=1&b=2": page}}, "research-a")
	collector.now = func() time.Time { return time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC) }
	first, firstMetrics := collector.Collect(context.Background(), SearchQuery{Text: "consulta uno"}, []scraper.SearchResult{{Title: "resultado", URL: "HTTPS://Example.COM:443/r?b=2&a=1"}})
	second, secondMetrics := collector.Collect(context.Background(), SearchQuery{Text: "consulta dos"}, []scraper.SearchResult{{Title: "resultado", URL: "https://example.com/r?a=1&b=2#seccion"}})
	if len(first) != 1 || len(second) != 0 {
		t.Fatalf("expected one canonical evidence item, got first=%d second=%d", len(first), len(second))
	}
	if secondMetrics.Duplicates != 1 || firstMetrics.PagesDistilled != 1 {
		t.Fatalf("unexpected metrics: first=%+v second=%+v", firstMetrics, secondMetrics)
	}
	items := collector.Evidence()
	if items[0].CanonicalURL != "https://example.com/r?a=1&b=2" || len(items[0].SourceQueries) != 2 {
		t.Fatalf("canonical provenance was not retained: %+v", items[0])
	}
	if items[0].ID == "" || items[0].ContentHash == "" || items[0].Type != EvidenceTypeFullText {
		t.Fatalf("missing evidence identity or classification: %+v", items[0])
	}
}

func TestEvidenceCollectorKeepsFailedDownloadAsSnippet(t *testing.T) {
	collector := NewEvidenceCollector(evidenceFetcher{err: errors.New("network unavailable")}, "research-b")
	items, metrics := collector.Collect(context.Background(), SearchQuery{Text: "consulta"}, []scraper.SearchResult{{Title: "Título", URL: "https://example.org/source", Content: "extracto parcial"}})
	if len(items) != 1 {
		t.Fatalf("expected snippet evidence, got %d", len(items))
	}
	item := items[0]
	if item.Type != EvidenceTypeSnippet || item.ExtractionStatus != ExtractionStatusFetchFailed {
		t.Fatalf("download failure was misclassified: %+v", item)
	}
	if metrics.SnippetsUsed != 1 || metrics.Failures[string(ExtractionStatusFetchFailed)] != 1 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
	if status, err := EvidenceSufficiency(items, DefaultEvidencePolicy()); err == nil || status != ResearchStatusFailed {
		t.Fatalf("one snippet must not satisfy minimum evidence: status=%s err=%v", status, err)
	}
}

func TestEvidenceSufficiencyDegradesSnippetOnlyCorpus(t *testing.T) {
	items := []Evidence{{CanonicalURL: "https://a.example", Type: EvidenceTypeSnippet}, {CanonicalURL: "https://b.example", Type: EvidenceTypeSnippet}, {CanonicalURL: "https://c.example", Type: EvidenceTypeSnippet}}
	status, err := EvidenceSufficiency(items, DefaultEvidencePolicy())
	if err != nil || status != ResearchStatusDegraded {
		t.Fatalf("expected degraded snippet-only corpus, got status=%s err=%v", status, err)
	}
}
