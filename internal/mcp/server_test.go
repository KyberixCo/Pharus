package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/KyberixCo/Pharus/internal/config"
)

func TestMCPServerHandlers(t *testing.T) {
	cfg := config.DefaultConfig()
	server := NewServer(cfg, nil)

	// Test MRTR handleAskUserInput
	tokenInfo := &TokenInfo{UserID: "usr_test_handler"}
	ctx := ContextWithTokenInfo(context.Background(), tokenInfo)
	req := &mcp.CallToolRequest{}
	_, mrtrRes, err := server.handleAskUserInput(ctx, req, UserInputRequest{
		Prompt: "¿Confirma proceder con la búsqueda?",
		Type:   "confirmation",
	})

	if err != nil {
		t.Fatalf("handleAskUserInput retornó error inesperado: %v", err)
	}

	if mrtrRes.Status != "input_required" {
		t.Errorf("Esperado status 'input_required', obtenido: %s", mrtrRes.Status)
	}
	if len(mrtrRes.InputRequests) != 1 || mrtrRes.InputRequests[0].Type != InputTypeConfirmation {
		t.Errorf("InputRequests inválido: %+v", mrtrRes.InputRequests)
	}

	// Test handleStartAsyncResearch
	_, asyncOut, err := server.handleStartAsyncResearch(ctx, req, StartAsyncResearchInput{
		Topic: "Inteligencia Artificial en Go",
	})
	if err != nil {
		t.Fatalf("handleStartAsyncResearch retornó error inesperado: %v", err)
	}

	if asyncOut.TaskID == "" || asyncOut.URI == "" {
		t.Errorf("Salida de tarea asíncrona inválida: %+v", asyncOut)
	}

	// Test Resource Reading for Task
	readReq := &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{
			URI: asyncOut.URI,
		},
	}
	resResult, err := server.handleReadTaskResource(ctx, readReq)
	if err != nil {
		t.Fatalf("handleReadTaskResource falló: %v", err)
	}
	if len(resResult.Contents) != 1 || resResult.Contents[0].URI != asyncOut.URI {
		t.Errorf("Contenido del recurso devuelto incorrecto: %+v", resResult)
	}

	// Test Resource Reading for Task List
	listReq := &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{
			URI: "resource://tasks/list",
		},
	}
	listResult, err := server.handleListTasksResource(ctx, listReq)
	if err != nil {
		t.Fatalf("handleListTasksResource falló: %v", err)
	}
	var tasks []*ResearchTask
	if err := json.Unmarshal([]byte(listResult.Contents[0].Text), &tasks); err != nil {
		t.Fatalf("Error al desmenuzar lista de tareas: %v", err)
	}
	if len(tasks) == 0 {
		t.Error("Se esperaba al menos 1 tarea en la lista de recursos")
	}

	// Test HTTP Handlers integration con auth
	mux := http.NewServeMux()
	server.RegisterHandlersWithAuth(mux, func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			info := &TokenInfo{UserID: "usr_test_handler"}
			next(w, r.WithContext(ContextWithTokenInfo(r.Context(), info)))
		}
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Test GET /mcp/subscriptions/listen (initial SSE response)
	reqSSE, _ := http.NewRequest("GET", ts.URL+"/mcp/subscriptions/listen", nil)
	ctxCancel, cancel := context.WithTimeout(ContextWithTokenInfo(context.Background(), tokenInfo), 200*time.Millisecond)
	defer cancel()
	reqSSE = reqSSE.WithContext(ctxCancel)

	resp, err := http.DefaultClient.Do(reqSSE)
	if err == nil {
		if resp.Header.Get("Content-Type") != "text/event-stream" {
			t.Errorf("Esperado Content-Type text/event-stream, obtenido %s", resp.Header.Get("Content-Type"))
		}
		resp.Body.Close()
	}
}
