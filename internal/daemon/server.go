package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/KyberixCo/Pharus/internal/config"
	"github.com/KyberixCo/Pharus/internal/i18n"
	"github.com/KyberixCo/Pharus/internal/mcp"
	"github.com/KyberixCo/Pharus/internal/research"
	"github.com/KyberixCo/Pharus/pkg/logger"
)

type Server struct {
	cfg         *config.Config
	engine      *research.Engine
	httpServer  *http.Server
	udsServer   *http.Server
	udsListener net.Listener
	mu          sync.Mutex
	startTime   time.Time
}

type ResearchRequest struct {
	Topic    string `json:"topic"`
	Language string `json:"language,omitempty"`
	Profile  string `json:"profile,omitempty"`
}

type ResearchResponse struct {
	ResearchID    string   `json:"research_id,omitempty"`
	Status        string   `json:"status"`
	Report        string   `json:"report,omitempty"`
	Error         string   `json:"error,omitempty"`
	FailureCode   string   `json:"failure_code,omitempty"`
	EvidenceCount int      `json:"evidence_count"`
	Warnings      []string `json:"warnings,omitempty"`
	Elapsed       string   `json:"elapsed"`
}

func NewServer(cfg *config.Config, engine *research.Engine) *Server {
	return &Server{
		cfg:       cfg,
		engine:    engine,
		startTime: time.Now(),
	}
}

func (s *Server) Start(ctx context.Context) error {
	log := logger.Get()
	log.Info("Starting Pharus Daemon Server...", "version", "1.0.0", "socket", s.cfg.SocketPath, "http", fmt.Sprintf("%s:%d", s.cfg.HTTPHost, s.cfg.HTTPPort))

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/research", s.authMiddleware(s.handleResearch))

	// Registrar servidor MCP Oficial Stateless
	mcpServer := mcp.NewServer(s.cfg, s.engine)
	mcpServer.RegisterHandlersWithAuth(mux, s.authMiddleware)

	// Clean existing socket
	_ = os.Remove(s.cfg.SocketPath)

	udsListener, err := net.Listen("unix", s.cfg.SocketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on UDS %s: %w", s.cfg.SocketPath, err)
	}
	_ = os.Chmod(s.cfg.SocketPath, 0600)
	s.udsListener = udsListener

	s.udsServer = &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // UDS caller context controls long research cancellation.
	}

	tcpAddr := fmt.Sprintf("%s:%d", s.cfg.HTTPHost, s.cfg.HTTPPort)
	s.httpServer = &http.Server{
		Addr:        tcpAddr,
		Handler:     mux,
		ReadTimeout: 30 * time.Second,
		// Research and MCP async/SSE responses have operation-specific
		// cancellation; a server-wide deadline would abort valid deep runs.
		WriteTimeout: 0,
	}

	errChan := make(chan error, 2)

	go func() {
		log.Info("UDS Server listening", "path", s.cfg.SocketPath)
		if err := s.udsServer.Serve(s.udsListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- fmt.Errorf("UDS server error: %w", err)
		}
	}()

	go func() {
		log.Info("HTTP Server listening", "addr", tcpAddr)
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- fmt.Errorf("HTTP server error: %w", err)
		}
	}()

	// Signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errChan:
		log.Error("Daemon server encountered fatal error", "error", err)
		return err
	case sig := <-sigChan:
		log.Info("Received shutdown signal", "signal", sig.String())
	case <-ctx.Done():
		log.Info("Context cancelled, shutting down daemon...")
	}

	return s.Stop()
}

func (s *Server) Stop() error {
	log := logger.Get()
	log.Info("Shutting down daemon servers gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if s.udsServer != nil {
		_ = s.udsServer.Shutdown(shutdownCtx)
	}
	if s.httpServer != nil {
		_ = s.httpServer.Shutdown(shutdownCtx)
	}
	_ = os.Remove(s.cfg.SocketPath)

	log.Info("Pharus Daemon Server stopped successfully")
	return nil
}

func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Control OOM: Limitar el tamaño del cuerpo de lectura dinámicamente según la ruta
		maxMB := 5
		if s.cfg != nil && s.cfg.Security.MaxRequestBodyMB > 0 {
			maxMB = s.cfg.Security.MaxRequestBodyMB
		}
		if strings.HasPrefix(r.URL.Path, "/mcp/subscriptions/listen") || r.Header.Get("Accept") == "text/event-stream" {
			if s.cfg != nil && s.cfg.Security.MaxSSEBufferMB > 0 {
				maxMB = s.cfg.Security.MaxSSEBufferMB
			} else {
				maxMB = 10
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, int64(maxMB)*1024*1024)

		isLoopback := r.RemoteAddr == "@" || r.URL.Scheme == "unix" ||
			strings.HasPrefix(r.RemoteAddr, "127.0.0.1") ||
			strings.HasPrefix(r.RemoteAddr, "[::1]") ||
			strings.HasPrefix(r.RemoteAddr, "localhost")

		// Conexiones UDS / Loopback local bypass token check pero inyectan TokenInfo
		if r.RemoteAddr == "@" || r.URL.Scheme == "unix" {
			userID := r.Header.Get("X-Pharus-User-ID")
			if userID == "" {
				userID = "user_local_default"
			}
			info := &mcp.TokenInfo{
				UserID:     userID,
				IsLoopback: true,
			}
			ctx := mcp.ContextWithTokenInfo(r.Context(), info)
			next(w, r.WithContext(ctx))
			return
		}

		token := r.Header.Get("X-Pharus-Token")
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		if s.cfg != nil && s.cfg.DaemonToken != "" && token != s.cfg.DaemonToken {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// Hardening: Pasaje estricto de identidad del usuario (TokenInfo.UserID)
		userID := r.Header.Get("X-Pharus-User-ID")
		if userID == "" && isLoopback {
			userID = "user_local_default"
		}

		info := &mcp.TokenInfo{
			Token:      token,
			UserID:     userID,
			IsLoopback: isLoopback,
		}
		ctx := mcp.ContextWithTokenInfo(r.Context(), info)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"service": "pharus-daemon",
		"time":    time.Now().Format(time.RFC3339),
		"uptime":  time.Since(s.startTime).String(),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":         "running",
		"uptime":         time.Since(s.startTime).String(),
		"llm_provider":   s.cfg.LLM.Provider,
		"minimax_model":  s.cfg.MiniMax.Model,
		"vector_db":      "chromem-go",
		"embed_provider": s.cfg.Embed.Provider,
	})
}

func (s *Server) handleResearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req ResearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Topic == "" {
		http.Error(w, `{"error":"invalid request, topic required"}`, http.StatusBadRequest)
		return
	}

	start := time.Now()
	log := logger.Get()
	log.Info("research request accepted via daemon")
	if s.engine == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(ResearchResponse{
			Status:      string(research.ResearchStatusFailed),
			Error:       "motor de investigación no inicializado",
			FailureCode: string(research.FailureCodeEngineUnavailable),
			Elapsed:     time.Since(start).String(),
		})
		return
	}

	requestedLanguage := req.Language
	if requestedLanguage == "" && s.cfg != nil {
		requestedLanguage = s.cfg.Language
	}
	language, languageErr := i18n.Resolve(requestedLanguage, nil)
	if languageErr != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": languageErr.Error()})
		return
	}
	profileValue := req.Profile
	if profileValue == "" && s.cfg != nil {
		profileValue = s.cfg.Research.DefaultProfile
	}
	profile, profileErr := research.ParseProfile(profileValue)
	if profileErr != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": profileErr.Error()})
		return
	}
	requestCtx := research.WithProfile(i18n.WithLanguage(r.Context(), language), profile)
	result, err := s.engine.ExecuteResearchResult(requestCtx, req.Topic)
	elapsed := time.Since(start).String()

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		if result == nil {
			result = &research.ResearchResult{Status: research.ResearchStatusFailed, FailureCode: research.FailureCodeOf(err)}
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(ResearchResponse{
			ResearchID:    result.ResearchID,
			Status:        string(research.ResearchStatusFailed),
			Error:         err.Error(),
			FailureCode:   string(research.FailureCodeOf(err)),
			EvidenceCount: result.EvidenceCount,
			Warnings:      result.Warnings,
			Elapsed:       elapsed,
		})
		return
	}

	_ = json.NewEncoder(w).Encode(ResearchResponse{
		ResearchID:    result.ResearchID,
		Status:        string(result.Status),
		Report:        result.Report,
		EvidenceCount: result.EvidenceCount,
		Warnings:      result.Warnings,
		Elapsed:       elapsed,
	})
}
