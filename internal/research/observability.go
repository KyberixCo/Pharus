package research

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/KyberixCo/Pharus/internal/scraper"
	"github.com/KyberixCo/Pharus/pkg/logger"
)

// researchObserver emits correlation-friendly, structured events without
// retaining topics, search queries, page contents, URLs, credentials, or raw
// upstream response bodies.
type researchObserver struct {
	log        *slog.Logger
	researchID string
	durations  map[string]int64
}

type researchObserverContextKey struct{}

func newResearchObserver(log *slog.Logger, researchID string) *researchObserver {
	return &researchObserver{log: log.With("research_id", researchID), researchID: researchID, durations: map[string]int64{}}
}

func withResearchObserver(ctx context.Context, observer *researchObserver) context.Context {
	return context.WithValue(ctx, researchObserverContextKey{}, observer)
}

// progressOperation exposes progress within long phases and emits a heartbeat
// every 30 seconds so CLI users can distinguish slow upstream work from a hang.
func progressOperation(ctx context.Context, phase, operation string, attrs ...any) func(error) {
	log := loggerForResearchContext(ctx)
	started := time.Now()
	baseAttrs := append([]any{"phase", phase, "operation", operation}, attrs...)
	log.Info("research operation started", baseAttrs...)

	done := make(chan struct{})
	var once sync.Once
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				heartbeatAttrs := append(append([]any{}, baseAttrs...), "elapsed_ms", time.Since(started).Milliseconds())
				log.Info("research operation still running", heartbeatAttrs...)
			}
		}
	}()

	return func(err error) {
		once.Do(func() {
			close(done)
			outcome := "success"
			if err != nil {
				outcome = "failed"
			}
			completedAttrs := append(append([]any{}, baseAttrs...),
				"duration_ms", time.Since(started).Milliseconds(),
				"outcome", outcome,
			)
			if err != nil {
				completedAttrs = append(completedAttrs, "error_kind", observableErrorKind(err))
			}
			log.Info("research operation completed", completedAttrs...)
		})
	}
}

func loggerForResearchContext(ctx context.Context) *slog.Logger {
	if observer, ok := ctx.Value(researchObserverContextKey{}).(*researchObserver); ok && observer != nil {
		return observer.log
	}
	return logger.Get()
}

func (o *researchObserver) phase(name string) func(error) {
	started := time.Now()
	o.log.Info("research phase started", "phase", name)
	return func(err error) {
		duration := time.Since(started).Milliseconds()
		o.durations[name] += duration
		outcome := "success"
		if err != nil {
			outcome = "failed"
		}
		attrs := []any{"phase", name, "duration_ms", duration, "outcome", outcome}
		if err != nil {
			attrs = append(attrs, "error_kind", observableErrorKind(err))
		}
		o.log.Info("research phase completed", attrs...)
	}
}

func (o *researchObserver) summary(result *ResearchResult, err error) {
	attrs := []any{
		"status", result.Status,
		"evidence_count", result.EvidenceCount,
		"warnings_count", len(result.Warnings),
		"failure_code", result.FailureCode,
		"durations_ms", result.PhaseDurations,
		"search_results", result.EvidenceMetrics.ResultsFound,
		"pages_downloaded", result.EvidenceMetrics.PagesDownloaded,
		"pages_distilled", result.EvidenceMetrics.PagesDistilled,
		"snippets_used", result.EvidenceMetrics.SnippetsUsed,
		"duplicates", result.EvidenceMetrics.Duplicates,
	}
	if err != nil {
		attrs = append(attrs, "error_kind", observableErrorKind(err))
	}
	o.log.Info("research workflow completed", attrs...)
}

func observableErrorKind(err error) string {
	if err == nil {
		return "invalid_output"
	}
	var httpErr *scraper.SearXNGHTTPError
	if errors.As(err, &httpErr) {
		return fmt.Sprintf("searxng_http_%d", httpErr.StatusCode)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	var languageErr *LanguageValidationError
	if errors.As(err, &languageErr) {
		return "language_invalid"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "status 529") || strings.Contains(message, "overload"):
		return "llm_overloaded"
	case strings.Contains(message, "status 429") || strings.Contains(message, "rate limit"):
		return "llm_rate_limited"
	case strings.Contains(message, "llm") || strings.Contains(message, "minimax") || strings.Contains(message, "openai") || strings.Contains(message, "anthropic"):
		return "llm_failed"
	case strings.Contains(message, "extraction") || strings.Contains(message, "fetch"):
		return "extraction_failed"
	case strings.Contains(message, "embed"):
		return "embeddings_failed"
	case strings.Contains(message, "index"):
		return "indexing_failed"
	case strings.Contains(message, "citation"):
		return "citations_invalid"
	default:
		return "operation_failed"
	}
}
