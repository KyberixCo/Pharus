package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/KyberixCo/Pharus/internal/config"
	"github.com/KyberixCo/Pharus/internal/i18n"
	"github.com/KyberixCo/Pharus/internal/research"
	"github.com/KyberixCo/Pharus/pkg/logger"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	cfg          *config.Config
	engine       *research.Engine
	mcpServer    *mcp.Server
	handler      *mcp.StreamableHTTPHandler
	sandbox      *SandboxRunner
	sandboxErr   error
	taskRegistry *TaskRegistry
	subManager   *SubscriptionManager
}

func NewServer(cfg *config.Config, engine *research.Engine) *Server {
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    "Pharus Deep Research Engine",
		Version: "1.0.0",
	}, nil)

	var sandbox *SandboxRunner
	var sandboxErr error
	if cfg != nil {
		sandbox, sandboxErr = NewSandboxRunnerWithConfig(cfg.Sandbox)
	} else {
		sandbox, sandboxErr = NewSandboxRunner("")
	}
	subMgr := NewSubscriptionManager()
	taskReg := NewTaskRegistry(subMgr)

	s := &Server{
		cfg:          cfg,
		engine:       engine,
		mcpServer:    mcpServer,
		sandbox:      sandbox,
		sandboxErr:   sandboxErr,
		taskRegistry: taskReg,
		subManager:   subMgr,
	}

	s.registerTools()
	s.registerResources()

	// Handler HTTP Stateless para el protocolo MCP 2026-07-28+
	opts := &mcp.StreamableHTTPOptions{
		Stateless: true, // Modo sin estado conforme a Diseño.md
	}

	s.handler = mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return s.mcpServer
	}, opts)

	return s
}

func (s *Server) registerTools() {
	// Tool 1: deep_research
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "deep_research",
		Description: "Ejecuta una investigación profunda síncrona multi-paso sobre un tema utilizando Co-STORM y DataSTORM",
	}, s.handleDeepResearch)

	// Tool 2: start_async_research
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "start_async_research",
		Description: "Inicia una investigación profunda asíncrona en segundo plano y retorna el URI del recurso (resource://tasks/{id})",
	}, s.handleStartAsyncResearch)

	// Tool 3: vector_search
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "vector_search",
		Description: "Realiza una búsqueda semántica de alta precisión en la base de conocimientos vectorial local",
	}, s.handleVectorSearch)

	// Tool 4: ask_user_input (MRTR / SEP-2322)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "ask_user_input",
		Description: "Solicita confirmación, aclaración o credenciales al usuario mediante Petición Multi-Vuelta (MRTR - SEP-2322)",
	}, s.handleAskUserInput)

	// Tool 5: execute_code_script (Code Execution MCP)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "execute_code_script",
		Description: "Ejecuta un script en Python o Bash dentro del entorno sandbox aislado del agente",
	}, s.handleExecuteCodeScript)
}

func (s *Server) registerResources() {
	// Recurso 1: Plantilla de detalles de tarea por ID (resource://tasks/{id})
	s.mcpServer.AddResourceTemplate(&mcp.ResourceTemplate{
		Name:        "task_details",
		Title:       "Detalles y Estado de Tarea Asíncrona",
		Description: "Obtiene el estado, progreso y reporte final de una tarea de investigación por ID",
		URITemplate: "resource://tasks/{id}",
		MIMEType:    "application/json",
	}, s.handleReadTaskResource)

	// Recurso 2: Lista general de tareas (resource://tasks/list)
	s.mcpServer.AddResource(&mcp.Resource{
		Name:        "tasks_list",
		Title:       "Lista de Tareas Asíncronas",
		Description: "Lista todas las tareas de investigación asíncronas registradas",
		URI:         "resource://tasks/list",
		MIMEType:    "application/json",
	}, s.handleListTasksResource)
}

func (s *Server) RegisterHandlers(mux *http.ServeMux) {
	mux.Handle("/mcp", s.handler)
	mux.Handle("/mcp/", s.handler)
	mux.HandleFunc("/mcp/subscriptions/listen", s.subManager.HandleListen)
}

func (s *Server) RegisterHandlersWithAuth(mux *http.ServeMux, authMw func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("/mcp", authMw(s.handler.ServeHTTP))
	mux.HandleFunc("/mcp/", authMw(s.handler.ServeHTTP))
	mux.HandleFunc("/mcp/subscriptions/listen", authMw(s.subManager.HandleListen))
}

func (s *Server) handleDeepResearch(ctx context.Context, req *mcp.CallToolRequest, input DeepResearchInput) (*mcp.CallToolResult, DeepResearchOutput, error) {
	log := logger.Get()
	log.Info("MCP Call: deep_research")

	tokenInfo, ok := TokenInfoFromContext(ctx)
	if !ok || tokenInfo == nil || tokenInfo.UserID == "" {
		log.Warn("MCP Call rejected: missing or unauthorized TokenInfo.UserID", "tool", "deep_research")
		return nil, DeepResearchOutput{
			Status: "error",
			Report: "Acceso denegado: Se requiere TokenInfo.UserID válido para ejecutar deep_research",
		}, fmt.Errorf("unauthorized: missing or invalid TokenInfo.UserID")
	}

	start := time.Now()
	var result *research.ResearchResult
	var err error

	if s.engine != nil {
		requestedLanguage := input.Language
		if requestedLanguage == "" && s.cfg != nil {
			requestedLanguage = s.cfg.Language
		}
		language, resolveErr := i18n.Resolve(requestedLanguage, nil)
		if resolveErr != nil {
			return nil, DeepResearchOutput{}, resolveErr
		}
		profileValue := input.Profile
		if profileValue == "" && s.cfg != nil {
			profileValue = s.cfg.Research.DefaultProfile
		}
		profile, profileErr := research.ParseProfile(profileValue)
		if profileErr != nil {
			return nil, DeepResearchOutput{}, profileErr
		}
		requestCtx := research.WithPrincipal(research.WithProfile(i18n.WithLanguage(ctx, language), profile), tokenInfo.UserID)
		result, err = s.engine.ExecuteResearchResult(requestCtx, input.Topic)
	} else {
		err = &research.ResearchError{
			Code: research.FailureCodeEngineUnavailable,
			Err:  fmt.Errorf("motor no inicializado para el tema: %s", input.Topic),
		}
	}

	elapsed := time.Since(start).String()
	if err != nil {
		failureCode := research.FailureCodeOf(err)
		researchID := ""
		if result != nil {
			researchID = result.ResearchID
		}
		return nil, DeepResearchOutput{
			ResearchID:  researchID,
			Status:      string(research.ResearchStatusFailed),
			Error:       err.Error(),
			FailureCode: string(failureCode),
			Elapsed:     elapsed,
		}, nil
	}

	return nil, DeepResearchOutput{
		ResearchID:    result.ResearchID,
		Status:        string(result.Status),
		Report:        result.Report,
		EvidenceCount: result.EvidenceCount,
		Warnings:      result.Warnings,
		Elapsed:       elapsed,
	}, nil
}

func (s *Server) handleStartAsyncResearch(ctx context.Context, req *mcp.CallToolRequest, input StartAsyncResearchInput) (*mcp.CallToolResult, StartAsyncResearchOutput, error) {
	log := logger.Get()
	log.Info("MCP Call: start_async_research", "topic", input.Topic)

	tokenInfo, ok := TokenInfoFromContext(ctx)
	if !ok || tokenInfo == nil || tokenInfo.UserID == "" {
		log.Warn("MCP Call rejected: missing or unauthorized TokenInfo.UserID", "tool", "start_async_research")
		return nil, StartAsyncResearchOutput{
			Status:  "error",
			Message: "Acceso denegado: Se requiere TokenInfo.UserID válido para iniciar investigaciones asíncronas",
		}, fmt.Errorf("unauthorized: missing or invalid TokenInfo.UserID")
	}

	requestedLanguage := input.Language
	if requestedLanguage == "" && s.cfg != nil {
		requestedLanguage = s.cfg.Language
	}
	language, resolveErr := i18n.Resolve(requestedLanguage, nil)
	if resolveErr != nil {
		return nil, StartAsyncResearchOutput{}, resolveErr
	}
	profileValue := input.Profile
	if profileValue == "" && s.cfg != nil {
		profileValue = s.cfg.Research.DefaultProfile
	}
	profile, profileErr := research.ParseProfile(profileValue)
	if profileErr != nil {
		return nil, StartAsyncResearchOutput{}, profileErr
	}

	userID := tokenInfo.UserID
	taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())
	task := s.taskRegistry.CreateTask(taskID, input.Topic, userID)
	taskURI := task.URI

	// Iniciar ejecución asíncrona
	go func(tID string, topic string, language i18n.Language, profile research.Profile) {
		bgCtx := research.WithPrincipal(research.WithProfile(i18n.WithLanguage(context.Background(), language), profile), userID)
		s.taskRegistry.UpdateTaskProgress(tID, 10, "searxng_search", "Iniciando búsqueda neural SearXNG")

		if s.engine == nil {
			s.taskRegistry.UpdateTaskStatus(tID, TaskStatusFailed, "", "Motor de investigación no inicializado")
			return
		}

		s.taskRegistry.UpdateTaskProgress(tID, 40, "costorm_discourse", "Ejecutando orquestación de discurso Co-STORM")
		result, err := s.engine.ExecuteResearchResult(bgCtx, topic)

		if err != nil {
			code := research.FailureCodeOf(err)
			message := err.Error()
			if code != "" {
				message = fmt.Sprintf("%s: %s", code, message)
			}
			s.taskRegistry.UpdateTaskStatus(tID, TaskStatusFailed, "", message)
		} else {
			s.taskRegistry.UpdateTaskProgress(tID, 90, "synthesis", "Sintetizando informe final en markdown")
			s.taskRegistry.UpdateTaskStatus(tID, TaskStatusCompleted, result.Report, "")
		}
	}(taskID, input.Topic, language, profile)

	return nil, StartAsyncResearchOutput{
		TaskID:  taskID,
		URI:     taskURI,
		Status:  string(TaskStatusRunning),
		Message: fmt.Sprintf("Tarea asíncrona iniciada. Consulte el progreso en el recurso URI: %s", taskURI),
	}, nil
}

func (s *Server) handleVectorSearch(ctx context.Context, req *mcp.CallToolRequest, input VectorSearchInput) (*mcp.CallToolResult, VectorSearchOutput, error) {
	log := logger.Get()
	log.Info("MCP Call: vector_search", "query", input.Query)

	tokenInfo, ok := TokenInfoFromContext(ctx)
	if !ok || tokenInfo == nil || tokenInfo.UserID == "" {
		return nil, VectorSearchOutput{}, fmt.Errorf("unauthorized: missing or invalid TokenInfo.UserID")
	}
	topK := input.TopK
	if topK <= 0 {
		topK = 5
	}

	var items []VectorSearchResultItem
	if s.engine == nil {
		return nil, VectorSearchOutput{}, fmt.Errorf("research engine is not initialized")
	}
	filter := map[string]string{"user_id": tokenInfo.UserID}
	if input.ResearchID != "" {
		filter["research_id"] = input.ResearchID
	}
	results, err := s.engine.SearchEvidenceFiltered(ctx, input.Query, topK, filter)
	if err != nil {
		return nil, VectorSearchOutput{}, fmt.Errorf("vector search failed: %w", err)
	}
	for _, result := range results {
		items = append(items, VectorSearchResultItem{
			ID:         result.ID,
			Content:    result.Content,
			Metadata:   result.Metadata,
			Similarity: result.Similarity,
		})
	}

	return nil, VectorSearchOutput{
		Count:   len(items),
		Results: items,
	}, nil
}

func (s *Server) handleAskUserInput(ctx context.Context, req *mcp.CallToolRequest, input UserInputRequest) (*mcp.CallToolResult, InputRequiredResult, error) {
	log := logger.Get()
	log.Info("MCP Call: ask_user_input (MRTR / SEP-2322)", "prompt", input.Prompt)

	reqType := InputTypePrompt
	if input.Type != "" {
		reqType = InputRequestType(input.Type)
	}

	// Devolver estructura nativa MRTR
	res := NewInputRequiredResult(
		"Se requiere interacción o datos del usuario para continuar la investigación",
		InputRequest{
			ID:       "user_input_request",
			Prompt:   input.Prompt,
			Type:     reqType,
			Required: true,
		},
	)

	return nil, res, nil
}

func (s *Server) handleExecuteCodeScript(ctx context.Context, req *mcp.CallToolRequest, input CodeExecutionInput) (*mcp.CallToolResult, CodeExecutionOutput, error) {
	log := logger.Get()
	log.Info("MCP Call: execute_code_script (Code Execution MCP)", "lang", input.Language)

	tokenInfo, ok := TokenInfoFromContext(ctx)
	if !ok || tokenInfo == nil || tokenInfo.UserID == "" {
		log.Warn("MCP Call rejected: missing or unauthorized TokenInfo.UserID", "tool", "execute_code_script")
		return nil, CodeExecutionOutput{
			Stderr:   "Acceso denegado: Se requiere TokenInfo.UserID válido para ejecutar scripts en el sandbox",
			ExitCode: -1,
		}, fmt.Errorf("unauthorized: missing or invalid TokenInfo.UserID")
	}

	if s.sandbox == nil {
		message := "Sandbox runner no está inicializado"
		if s.sandboxErr != nil {
			message = s.sandboxErr.Error()
		}
		return nil, CodeExecutionOutput{
			Stderr:   message,
			ExitCode: -1,
		}, nil
	}

	out, err := s.sandbox.ExecuteScript(ctx, input)
	if err != nil {
		return nil, CodeExecutionOutput{}, err
	}

	return nil, out, nil
}

func (s *Server) handleReadTaskResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	uri := req.Params.URI
	parts := strings.Split(uri, "/")
	taskID := parts[len(parts)-1]

	task, exists := s.taskRegistry.GetTask(taskID)
	if !exists {
		return nil, mcp.ResourceNotFoundError(uri)
	}

	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return nil, err
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      uri,
				MIMEType: "application/json",
				Text:     string(data),
			},
		},
	}, nil
}

func (s *Server) handleListTasksResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	userID, _ := ctx.Value("user_id").(string)
	tasks := s.taskRegistry.ListTasks(userID)

	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return nil, err
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		},
	}, nil
}

type DeepResearchInput struct {
	Topic    string `json:"topic" jsonschema:"Research topic or hypothesis to explore"`
	Language string `json:"language,omitempty" jsonschema:"Output language: auto, en, or es"`
	Profile  string `json:"profile,omitempty" jsonschema:"Effort profile: quick, balanced, or deep"`
}

type DeepResearchOutput struct {
	ResearchID    string   `json:"research_id,omitempty" jsonschema:"Identificador de correlación de la investigación"`
	Report        string   `json:"report,omitempty" jsonschema:"El informe completo generado en markdown"`
	Status        string   `json:"status" jsonschema:"Estado de finalización de la tarea"`
	Error         string   `json:"error,omitempty" jsonschema:"Motivo del fallo de investigación"`
	FailureCode   string   `json:"failure_code,omitempty" jsonschema:"Código estable del fallo de investigación"`
	EvidenceCount int      `json:"evidence_count" jsonschema:"Número de documentos de evidencia recuperados"`
	Warnings      []string `json:"warnings,omitempty" jsonschema:"Advertencias no fatales de la investigación"`
	Elapsed       string   `json:"elapsed" jsonschema:"Tiempo transcurrido en la investigación"`
}

type StartAsyncResearchInput struct {
	Topic    string `json:"topic" jsonschema:"Research topic or hypothesis to run asynchronously"`
	Language string `json:"language,omitempty" jsonschema:"Output language: auto, en, or es"`
	Profile  string `json:"profile,omitempty" jsonschema:"Effort profile: quick, balanced, or deep"`
}

type StartAsyncResearchOutput struct {
	TaskID  string `json:"task_id" jsonschema:"ID de la tarea asíncrona generada"`
	URI     string `json:"uri" jsonschema:"URI del recurso para monitoreo (resource://tasks/{id})"`
	Status  string `json:"status" jsonschema:"Estado inicial de la tarea"`
	Message string `json:"message" jsonschema:"Instrucciones de seguimiento"`
}

type VectorSearchInput struct {
	Query      string `json:"query" jsonschema:"Consulta semántica para buscar evidencia en la base vectorial"`
	TopK       int    `json:"top_k,omitempty" jsonschema:"Número de fragmentos vectoriales a recuperar"`
	ResearchID string `json:"research_id,omitempty" jsonschema:"Restringe la recuperación a una investigación concreta"`
}

type VectorSearchOutput struct {
	Count   int                      `json:"count"`
	Results []VectorSearchResultItem `json:"results"`
}

type VectorSearchResultItem struct {
	ID         string            `json:"id"`
	Content    string            `json:"content"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Similarity float32           `json:"similarity"`
}

type UserInputRequest struct {
	Prompt string `json:"prompt" jsonschema:"Pregunta o solicitud de confirmación para el usuario"`
	Type   string `json:"type,omitempty" jsonschema:"Tipo opcional de entrada requerida (prompt, confirmation, credentials)"`
}
