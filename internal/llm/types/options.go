package types

import (
	"context"
	"time"
)

// RequestOptions lets a workflow tune one expensive completion without
// changing the provider interface or the global defaults used by small calls.
type RequestOptions struct {
	MaxTokens   int
	MaxAttempts int
	Timeout     time.Duration
}

type requestOptionsContextKey struct{}

func WithRequestOptions(ctx context.Context, options RequestOptions) context.Context {
	return context.WithValue(ctx, requestOptionsContextKey{}, options)
}

func RequestOptionsFromContext(ctx context.Context) RequestOptions {
	if ctx == nil {
		return RequestOptions{}
	}
	options, _ := ctx.Value(requestOptionsContextKey{}).(RequestOptions)
	return options
}
