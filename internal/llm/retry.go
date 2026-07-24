package llm

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/KyberixCo/Pharus/internal/llm/types"
	"github.com/KyberixCo/Pharus/pkg/logger"
)

const (
	defaultRetryMaxAttempts = 5
	defaultRetryInitialWait = 2 * time.Second
	defaultRetryMaxWait     = 30 * time.Second
)

var llmHTTPStatusPattern = regexp.MustCompile(`(?i)(?:status|status code)\s*[=:]?\s*(\d{3})`)

type RetryPolicy struct {
	MaxAttempts int
	InitialWait time.Duration
	MaxWait     time.Duration
}

type correlationIDContextKey struct{}

// WithCorrelationID adds a non-sensitive workflow identifier to retry logs.
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, correlationIDContextKey{}, correlationID)
}

type retryingProvider struct {
	next   types.Provider
	policy RetryPolicy
}

func NewRetryingProvider(next types.Provider, policy RetryPolicy) types.Provider {
	if next == nil {
		return nil
	}
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = defaultRetryMaxAttempts
	}
	if policy.InitialWait <= 0 {
		policy.InitialWait = defaultRetryInitialWait
	}
	if policy.MaxWait <= 0 {
		policy.MaxWait = defaultRetryMaxWait
	}
	if policy.MaxWait < policy.InitialWait {
		policy.MaxWait = policy.InitialWait
	}
	return &retryingProvider{next: next, policy: policy}
}

func (p *retryingProvider) GenerateCompletion(ctx context.Context, messages []types.Message, temperature float64) (string, error) {
	var lastErr error
	attempts := 0
	wait := p.policy.InitialWait
	maxAttempts := p.policy.MaxAttempts
	if requested := types.RequestOptionsFromContext(ctx).MaxAttempts; requested > 0 && requested < maxAttempts {
		maxAttempts = requested
	}
	log := logger.Get()
	if correlationID, ok := ctx.Value(correlationIDContextKey{}).(string); ok && correlationID != "" {
		log = log.With("research_id", correlationID)
	}
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		attempts = attempt
		result, err := p.next.GenerateCompletion(ctx, messages, temperature)
		if err == nil {
			if attempt > 1 {
				log.Info("LLM request recovered after retry", "attempt", attempt)
			}
			return result, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		reason, retryable := transientLLMError(err)
		if !retryable || attempt == maxAttempts {
			break
		}

		log.Warn("transient LLM request failure; retry scheduled",
			"attempt", attempt,
			"max_attempts", maxAttempts,
			"retry_in_ms", wait.Milliseconds(),
			"error_kind", reason,
		)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return "", ctx.Err()
		case <-timer.C:
		}
		wait *= 2
		if wait > p.policy.MaxWait {
			wait = p.policy.MaxWait
		}
	}
	return "", fmt.Errorf("LLM request failed after %d attempt(s): %w", attempts, lastErr)
}

func transientLLMError(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	if errors.Is(err, context.Canceled) {
		return "canceled", false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return "timeout", true
		}
		if netErr.Temporary() {
			return "network_temporary", true
		}
	}

	message := strings.ToLower(err.Error())
	if match := llmHTTPStatusPattern.FindStringSubmatch(message); len(match) == 2 {
		switch match[1] {
		case "408", "409", "425", "429", "500", "502", "503", "504", "529":
			return "http_" + match[1], true
		default:
			return "http_" + match[1], false
		}
	}
	for kind, marker := range map[string]string{
		"overloaded":        "overload",
		"rate_limited":      "rate limit",
		"connection_reset":  "connection reset",
		"unexpected_eof":    "unexpected eof",
		"connection_closed": "server closed idle connection",
	} {
		if strings.Contains(message, marker) {
			return kind, true
		}
	}
	return "permanent", false
}

// TransientErrorKind reports whether retrying the same logical operation with
// a smaller request is appropriate.
func TransientErrorKind(err error) (string, bool) {
	return transientLLMError(err)
}
