package mcp

import (
	"context"
)

type ContextKey string

const (
	TokenInfoKey ContextKey = "pharus_token_info"
)

// TokenInfo representa la identidad verificada del usuario autenticado en Pharus.
type TokenInfo struct {
	Token      string `json:"token"`
	UserID     string `json:"user_id"`
	IsLoopback bool   `json:"is_loopback"`
}

// ContextWithTokenInfo inyecta TokenInfo de forma inmutable en el context.
func ContextWithTokenInfo(ctx context.Context, info *TokenInfo) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, TokenInfoKey, info)
	if info != nil && info.UserID != "" {
		ctx = context.WithValue(ctx, "user_id", info.UserID)
	}
	return ctx
}

// TokenInfoFromContext recupera TokenInfo desde el context con fallback a "user_id".
func TokenInfoFromContext(ctx context.Context) (*TokenInfo, bool) {
	if ctx == nil {
		return nil, false
	}
	if info, ok := ctx.Value(TokenInfoKey).(*TokenInfo); ok && info != nil {
		return info, true
	}
	if uid, ok := ctx.Value("user_id").(string); ok && uid != "" {
		return &TokenInfo{
			UserID: uid,
		}, true
	}
	return nil, false
}
