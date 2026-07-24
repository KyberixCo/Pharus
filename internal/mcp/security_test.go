package mcp

import (
	"context"
	"testing"

	"github.com/KyberixCo/Pharus/internal/config"
)

func TestMCP_ToolAuthorization_ExecuteCodeScript(t *testing.T) {
	cfg := config.DefaultConfig()
	server := NewServer(cfg, nil)

	input := CodeExecutionInput{
		Language: "python",
		Script:   "print('hello')",
	}

	// Case 1: Unauthenticated call without TokenInfo.UserID
	ctxNoAuth := context.Background()
	_, outNoAuth, errNoAuth := server.handleExecuteCodeScript(ctxNoAuth, nil, input)
	if errNoAuth == nil {
		t.Error("expected error for unauthenticated execute_code_script call")
	}
	if outNoAuth.ExitCode != -1 {
		t.Errorf("expected exit code -1, got %d", outNoAuth.ExitCode)
	}

	// Case 2: Authenticated call with TokenInfo.UserID
	tokenInfo := &TokenInfo{
		Token:  "test-token",
		UserID: "usr_alice",
	}
	ctxAuth := ContextWithTokenInfo(context.Background(), tokenInfo)
	_, outAuth, errAuth := server.handleExecuteCodeScript(ctxAuth, nil, input)
	if errAuth != nil {
		t.Errorf("unexpected error for authenticated call: %v", errAuth)
	}
	if outAuth.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", outAuth.ExitCode)
	}
}

func TestMCP_ToolAuthorization_AsyncResearch(t *testing.T) {
	cfg := config.DefaultConfig()
	server := NewServer(cfg, nil)

	input := StartAsyncResearchInput{
		Topic: "Quantum Computing Advances",
	}

	// Case 1: Without UserID
	ctxNoAuth := context.Background()
	_, outNoAuth, errNoAuth := server.handleStartAsyncResearch(ctxNoAuth, nil, input)
	if errNoAuth == nil {
		t.Error("expected error for unauthenticated start_async_research call")
	}
	if outNoAuth.Status != "error" {
		t.Errorf("expected status 'error', got %s", outNoAuth.Status)
	}

	// Case 2: With UserID
	tokenInfo := &TokenInfo{
		Token:  "test-token",
		UserID: "usr_bob",
	}
	ctxAuth := ContextWithTokenInfo(context.Background(), tokenInfo)
	_, outAuth, errAuth := server.handleStartAsyncResearch(ctxAuth, nil, input)
	if errAuth != nil {
		t.Errorf("unexpected error for authenticated call: %v", errAuth)
	}
	if outAuth.TaskID == "" {
		t.Error("expected valid TaskID for authenticated call")
	}
}

func TestMCP_ToolAuthorization_VectorSearch(t *testing.T) {
	server := NewServer(config.DefaultConfig(), nil)
	_, _, err := server.handleVectorSearch(context.Background(), nil, VectorSearchInput{Query: "private evidence"})
	if err == nil {
		t.Fatal("expected unauthenticated vector_search to be rejected")
	}
}
