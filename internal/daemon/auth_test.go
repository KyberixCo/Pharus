package daemon

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KyberixCo/Pharus/internal/config"
	"github.com/KyberixCo/Pharus/internal/mcp"
)

func TestAuthMiddleware_ValidTokenAndUserID(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DaemonToken = "secret-test-token"

	server := NewServer(cfg, nil)

	var capturedTokenInfo *mcp.TokenInfo
	handler := server.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		info, ok := mcp.TokenInfoFromContext(r.Context())
		if !ok {
			t.Error("expected TokenInfo in context")
		}
		capturedTokenInfo = info
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/mcp", nil)
	req.Header.Set("X-Pharus-Token", "secret-test-token")
	req.Header.Set("X-Pharus-User-ID", "usr_test123")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if capturedTokenInfo == nil {
		t.Fatal("capturedTokenInfo is nil")
	}
	if capturedTokenInfo.UserID != "usr_test123" {
		t.Errorf("expected UserID usr_test123, got %s", capturedTokenInfo.UserID)
	}
	if capturedTokenInfo.Token != "secret-test-token" {
		t.Errorf("expected Token secret-test-token, got %s", capturedTokenInfo.Token)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DaemonToken = "secret-test-token"

	server := NewServer(cfg, nil)

	handler := server.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/mcp", nil)
	req.Header.Set("X-Pharus-Token", "wrong-token")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestAuthMiddleware_OOMBodyLimit(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DaemonToken = "test-token"
	cfg.Security.MaxRequestBodyMB = 1 // 1 MB limit

	server := NewServer(cfg, nil)

	handler := server.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// 2 MB payload (exceeds 1 MB limit)
	largeData := bytes.Repeat([]byte("A"), 2*1024*1024)
	req := httptest.NewRequest("POST", "/research", bytes.NewReader(largeData))
	req.Header.Set("X-Pharus-Token", "test-token")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413, got %d", rec.Code)
	}
}
