package research

import "context"

type principalContextKey struct{}

// WithPrincipal associates an authenticated transport user with a research
// execution so indexed evidence can be isolated on later retrieval.
func WithPrincipal(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, principalContextKey{}, userID)
}

func principalFromContext(ctx context.Context) string {
	userID, _ := ctx.Value(principalContextKey{}).(string)
	return userID
}
