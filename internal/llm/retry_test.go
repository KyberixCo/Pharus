package llm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KyberixCo/Pharus/internal/llm/types"
)

type retryTestProvider struct {
	failures int
	err      error
	calls    int
}

func (p *retryTestProvider) GenerateCompletion(context.Context, []types.Message, float64) (string, error) {
	p.calls++
	if p.calls <= p.failures {
		return "", p.err
	}
	return "ok", nil
}

func TestRetryingProviderRecoversFromOverloaded529(t *testing.T) {
	upstream := &retryTestProvider{failures: 2, err: errors.New("MiniMax API error (status 529): overloaded_error")}
	provider := NewRetryingProvider(upstream, RetryPolicy{MaxAttempts: 4, InitialWait: time.Millisecond, MaxWait: time.Millisecond})

	result, err := provider.GenerateCompletion(context.Background(), nil, 0.2)
	if err != nil {
		t.Fatalf("expected transient failure to recover, got %v", err)
	}
	if result != "ok" || upstream.calls != 3 {
		t.Fatalf("expected success on third attempt, result=%q calls=%d", result, upstream.calls)
	}
}

func TestRetryingProviderDoesNotRetryPermanentError(t *testing.T) {
	upstream := &retryTestProvider{failures: 4, err: errors.New("MiniMax API error (status 401): unauthorized")}
	provider := NewRetryingProvider(upstream, RetryPolicy{MaxAttempts: 4, InitialWait: time.Millisecond, MaxWait: time.Millisecond})

	_, err := provider.GenerateCompletion(context.Background(), nil, 0.2)
	if err == nil {
		t.Fatal("expected permanent error")
	}
	if upstream.calls != 1 {
		t.Fatalf("expected one attempt for permanent error, got %d", upstream.calls)
	}
}

func TestRetryingProviderStopsWhenContextIsCanceled(t *testing.T) {
	upstream := &retryTestProvider{failures: 4, err: errors.New("OpenAI API error (status 503): unavailable")}
	provider := NewRetryingProvider(upstream, RetryPolicy{MaxAttempts: 4, InitialWait: time.Second, MaxWait: time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := provider.GenerateCompletion(ctx, nil, 0.2)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if upstream.calls != 1 {
		t.Fatalf("expected no retry after cancellation, got %d calls", upstream.calls)
	}
}

func TestRetryingProviderHonorsPerRequestAttemptBudget(t *testing.T) {
	upstream := &retryTestProvider{failures: 5, err: errors.New("MiniMax HTTP request failed: unexpected EOF")}
	provider := NewRetryingProvider(upstream, RetryPolicy{MaxAttempts: 5, InitialWait: time.Millisecond, MaxWait: time.Millisecond})
	ctx := types.WithRequestOptions(context.Background(), types.RequestOptions{MaxAttempts: 2})

	_, err := provider.GenerateCompletion(ctx, nil, 0.2)
	if err == nil {
		t.Fatal("expected request to fail within its attempt budget")
	}
	if upstream.calls != 2 {
		t.Fatalf("expected exactly two expensive attempts, got %d", upstream.calls)
	}
}
