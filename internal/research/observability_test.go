package research

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/KyberixCo/Pharus/internal/scraper"
)

func TestResearchObserverCorrelatesPhasesWithoutSensitiveValues(t *testing.T) {
	var output bytes.Buffer
	observer := newResearchObserver(slog.New(slog.NewJSONHandler(&output, nil)), "research_test_123")
	end := observer.phase("search")
	end(&scraper.SearXNGHTTPError{StatusCode: 403, Body: "token=secret-value"})
	result := &ResearchResult{
		ResearchID:     "research_test_123",
		Status:         ResearchStatusFailed,
		FailureCode:    FailureCodeSearchUnavailable,
		PhaseDurations: observer.durations,
	}
	observer.summary(result, errors.New("token=secret-value"))

	logs := output.String()
	for _, want := range []string{"research_id", "research_test_123", "\"phase\":\"search\"", "duration_ms", "searxng_http_403"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("logs missing %q: %s", want, logs)
		}
	}
	if strings.Contains(logs, "secret-value") {
		t.Fatalf("logs must not include raw error bodies or secrets: %s", logs)
	}
	if strings.Count(logs, `"outcome"`) != 1 {
		t.Fatalf("phase completion must contain exactly one outcome field: %s", logs)
	}
}
