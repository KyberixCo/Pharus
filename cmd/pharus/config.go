package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/KyberixCo/Pharus/internal/config"
	"github.com/KyberixCo/Pharus/internal/daemon"
	"github.com/KyberixCo/Pharus/internal/embedding"
	"github.com/KyberixCo/Pharus/internal/sysconfig"
	"github.com/spf13/cobra"
)

type MLXModelPreset struct {
	ID          string
	Repo        string
	RAM         string
	Languages   string
	Description string
}

var mlxPresets = []MLXModelPreset{
	{
		ID:          "Qwen3-Embedding-0.6B (8-bit)",
		Repo:        "mlx-community/Qwen3-Embedding-0.6B-8bit",
		RAM:         "~650 MB RAM",
		Languages:   "Multilingüe Avanzado (Español, Inglés, Código, 100+ idiomas)",
		Description: "🏆 Recomendado: Conversión MLX de Qwen3 con excelente precisión semántica multilingüe.",
	},
	{
		ID:          "ModernBERT Embed (4-bit)",
		Repo:        "mlx-community/nomicai-modernbert-embed-base-4bit",
		RAM:         "~600 MB RAM (Liviano)",
		Languages:   "Multilingüe (Español, Inglés, 8,192 tokens de contexto)",
		Description: "Excelente balance de velocidad y contexto largo para Deep Research.",
	},
	{
		ID:          "BGE-M3 Multilingual",
		Repo:        "mlx-community/bge-m3-mlx-8bit",
		RAM:         "~600 MB RAM",
		Languages:   "Multilingüe Avanzado (Español, Inglés, Francés, Alemán, 100+ idiomas)",
		Description: "Máxima precisión de búsqueda semántica multilingüe con dense & sparse retrieval.",
	},
	{
		ID:          "Snowflake Arctic Embed L v2.0",
		Repo:        "mlx-community/snowflake-arctic-embed-l-v2.0-4bit",
		RAM:         "~1.1 GB RAM",
		Languages:   "Multilingüe (Alta precisión empresarial)",
		Description: "Optimizado para documentos técnicos y legales complejos.",
	},
	{
		ID:          "BGE Small v1.5",
		Repo:        "mlx-community/bge-small-en-v1.5-8bit",
		RAM:         "~300 MB RAM (Micro)",
		Languages:   "Inglés / Términos técnicos globales",
		Description: "Ultra ligero para sistemas con recursos de RAM ajustados.",
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Asistente interactivo (wizard) completo para configurar LLMs, Embeddings (OMLX/MLX/Ollama/OpenAI), SearXNG e inicio automático",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("⚙️  Asistente de Configuración Interactivo de Pharus")
		fmt.Println("============================================================")

		reader := bufio.NewReader(os.Stdin)

		// ----------------------------------------------------------------------
		// STEP 1: Selección de Proveedor LLM
		// ----------------------------------------------------------------------
		fmt.Println("\n--- 1. Proveedor de LLM (Inferencia) ---")
		fmt.Println("  1) MiniMax (MiniMax-M3 / Anthropic-compatible API)")
		fmt.Println("  2) OpenAI (gpt-4o-mini / gpt-4o)")
		fmt.Println("  3) Anthropic (claude-3-5-sonnet)")
		fmt.Println("  4) Ollama (Local LLM)")
		fmt.Println("  5) llama.cpp (Local LLM con soporte GBNF)")

		currentProvider := cfg.LLM.Provider
		if currentProvider == "" {
			currentProvider = "minimax"
		}
		fmt.Printf("\n👉 Elige un proveedor de LLM [1-5] (actual: %s): ", currentProvider)
		optStr, _ := reader.ReadString('\n')
		optStr = strings.TrimSpace(optStr)

		switch optStr {
		case "2":
			cfg.LLM.Provider = "openai"
			fmt.Println("\n  [OpenAI Config]")
			cfg.OpenAI.APIKey = promptField(reader, "API Key de OpenAI", cfg.OpenAI.APIKey, true)
			cfg.OpenAI.Model = promptField(reader, "Modelo de OpenAI", "gpt-4o-mini", false)
			cfg.OpenAI.BaseURL = promptField(reader, "Base URL", "https://api.openai.com/v1", false)

		case "3":
			cfg.LLM.Provider = "anthropic"
			fmt.Println("\n  [Anthropic Config]")
			cfg.Anthropic.APIKey = promptField(reader, "API Key de Anthropic", cfg.Anthropic.APIKey, true)
			cfg.Anthropic.Model = promptField(reader, "Modelo de Anthropic", "claude-3-5-sonnet-20241022", false)
			cfg.Anthropic.BaseURL = promptField(reader, "Base URL", "https://api.anthropic.com/v1", false)

		case "4":
			cfg.LLM.Provider = "ollama"
			fmt.Println("\n  [Ollama Local Config]")
			cfg.OllamaLLM.BaseURL = promptField(reader, "URL Servidor Ollama", "http://localhost:11434", false)
			cfg.OllamaLLM.Model = promptField(reader, "Modelo Chat Ollama", "llama3.1", false)

		case "5":
			cfg.LLM.Provider = "llamacpp"
			fmt.Println("\n  [llama.cpp Local Config]")
			cfg.LlamaCPP.BaseURL = promptField(reader, "URL servidor llama.cpp", "http://localhost:8080", false)
			cfg.LlamaCPP.Model = promptField(reader, "Modelo", "qwen2.5-coder-7b-instruct", false)

		default:
			cfg.LLM.Provider = "minimax"
			fmt.Println("\n  [MiniMax Config]")
			cfg.MiniMax.APIKey = promptField(reader, "API Key de MiniMax", cfg.MiniMax.APIKey, true)
			cfg.MiniMax.GroupID = promptField(reader, "Group ID (opcional)", cfg.MiniMax.GroupID, false)
			cfg.MiniMax.Model = promptField(reader, "Modelo de MiniMax", "MiniMax-M3", false)
			cfg.MiniMax.BaseURL = promptField(reader, "Base URL", "https://api.minimax.io/anthropic", false)
		}

		// ----------------------------------------------------------------------
		// STEP 2: Selección de Proveedor de Embeddings (OMLX / MLX / Ollama / Externo)
		// ----------------------------------------------------------------------
		fmt.Println("\n--- 2. Proveedor de Embeddings (Vector Database) ---")
		fmt.Println("  1) OMLX (Open MLX Server - 'omlx serve' en Apple Silicon Metal - Recomendado)")
		fmt.Println("  2) MLX Framework (Python MLX Server - 'mlx_lm.server')")
		fmt.Println("  3) Ollama (Local Embeddings Server - 'ollama serve')")
		fmt.Println("  4) External API (OpenAI / Custom Embeddings API)")

		currentEmbed := cfg.Embed.Provider
		if currentEmbed == "" {
			currentEmbed = "omlx"
		}
		fmt.Printf("\n👉 Elige un proveedor de Embeddings [1-4] (actual: %s): ", currentEmbed)
		embedOpt, _ := reader.ReadString('\n')
		embedOpt = strings.TrimSpace(embedOpt)

		switch embedOpt {
		case "2":
			cfg.Embed.Provider = "mlx"
			fmt.Println("\n  [MLX Framework Embeddings Config]")
			cfg.Embed.URL = promptField(reader, "URL Servidor MLX", "http://localhost:8080", false)
			cfg.Embed.Model = promptField(reader, "Modelo MLX", "Qwen/Qwen3-Embedding-0.6B", false)
			validateEmbeddingServerImmediate(reader, cfg)

		case "3":
			cfg.Embed.Provider = "ollama"
			fmt.Println("\n  [Ollama Embeddings Config]")
			cfg.Embed.URL = promptField(reader, "URL Servidor Ollama", "http://localhost:11434", false)
			cfg.Embed.Model = promptField(reader, "Modelo Embeddings", "nomic-embed-text", false)
			validateEmbeddingServerImmediate(reader, cfg)

		case "4":
			cfg.Embed.Provider = "openai"
			fmt.Println("\n  [External / OpenAI Embeddings Config]")
			cfg.Embed.URL = promptField(reader, "Base URL Embeddings", "https://api.openai.com/v1", false)
			cfg.Embed.Model = promptField(reader, "Modelo Embeddings", "text-embedding-3-small", false)
			cfg.Embed.APIKey = promptField(reader, "API Key Embeddings (si requiere)", cfg.Embed.APIKey, true)
			validateEmbeddingServerImmediate(reader, cfg)

		default:
			cfg.Embed.Provider = "omlx"
			fmt.Println("\n  [OMLX (Open MLX) Embeddings Config - Aceleración Metal Apple Silicon]")
			fmt.Println("\n📌 Modelos de Embeddings Recomendados:")
			for i, p := range mlxPresets {
				fmt.Printf("  [%d] %-30s (%s)\n", i+1, p.ID, p.Repo)
				fmt.Printf("      • Memoria RAM: %s\n", p.RAM)
				fmt.Printf("      • Idiomas:     %s\n", p.Languages)
				fmt.Printf("      • Detalle:     %s\n\n", p.Description)
			}
			fmt.Println("  [6] Especificar modelo personalizado desde HuggingFace")

			fmt.Print("👉 Elige un modelo para OMLX [1-6] (default: 1 - Qwen3-Embedding-0.6B): ")
			mOpt, _ := reader.ReadString('\n')
			mOpt = strings.TrimSpace(mOpt)

			selectedRepo := mlxPresets[0].Repo
			switch mOpt {
			case "2":
				selectedRepo = mlxPresets[1].Repo
			case "3":
				selectedRepo = mlxPresets[2].Repo
			case "4":
				selectedRepo = mlxPresets[3].Repo
			case "5":
				selectedRepo = mlxPresets[4].Repo
			case "6":
				selectedRepo = promptField(reader, "Repositorio HuggingFace compatible con MLX / OMLX", mlxPresets[0].Repo, false)
			default:
				selectedRepo = mlxPresets[0].Repo
			}

			cfg.Embed.Model = selectedRepo

			defaultOmlxURL := cfg.Embed.URL
			if defaultOmlxURL == "" || strings.Contains(defaultOmlxURL, "11434") || strings.Contains(defaultOmlxURL, "8080") {
				defaultOmlxURL = "http://localhost:8000"
			}
			cfg.Embed.URL = promptField(reader, "URL Servidor OMLX", defaultOmlxURL, false)

			// El servicio se instala e inicia sin pasos adicionales cuando es local.
			validateEmbeddingServerImmediate(reader, cfg)
		}

		// ----------------------------------------------------------------------
		// STEP 3: Configuración de SearXNG (Meta-Search Engine)
		// ----------------------------------------------------------------------
		fmt.Println("\n--- 3. Servidor de Búsqueda Web SearXNG ---")
		cfg.Search.SearXNGURL = promptField(reader, "URL de SearXNG Meta-Search", cfg.Search.SearXNGURL, false)

		fmt.Print("👉 ¿Deseas verificar/desplegar un contenedor SearXNG local con Docker? [S/n]: ")
		deploySearx, _ := reader.ReadString('\n')
		deploySearx = strings.ToLower(strings.TrimSpace(deploySearx))
		if deploySearx == "" || deploySearx == "s" || deploySearx == "y" || deploySearx == "si" {
			deploySearXNGContainer()
		}

		// ----------------------------------------------------------------------
		// STEP 4: Fuente estructurada opcional para DataSTORM
		// ----------------------------------------------------------------------
		fmt.Println("\n--- 4. Datos estructurados DataSTORM (opcional) ---")
		fmt.Print("Ruta a un archivo CSV (Enter para conservar la configuración actual): ")
		dataPath, _ := reader.ReadString('\n')
		dataPath = strings.TrimSpace(dataPath)
		if dataPath != "" {
			absolutePath, absErr := filepath.Abs(dataPath)
			if absErr != nil {
				fmt.Printf("⚠️ Ruta DataSTORM inválida: %v\n", absErr)
			} else {
				tableName := promptField(reader, "Nombre de tabla SQL", "research_metrics", false)
				cfg.DataSTORM.Sources = map[string]string{tableName: absolutePath}
			}
		}

		// ----------------------------------------------------------------------
		// STEP 5: Inicio Automático del Demonio (launchd macOS)
		// ----------------------------------------------------------------------
		fmt.Println("\n--- 5. Servicio Demonio Pharus (Inicio Automático en macOS) ---")
		fmt.Print("👉 ¿Deseas registrar e iniciar el demonio Pharus en background (launchd)? [S/n]: ")
		startDaemon, _ := reader.ReadString('\n')
		startDaemon = strings.ToLower(strings.TrimSpace(startDaemon))
		if startDaemon == "" || startDaemon == "s" || startDaemon == "y" || startDaemon == "si" {
			fmt.Println("⚙️ Registrando e iniciando servicio launchd...")
			if err := daemon.InstallLaunchdService(cfg); err != nil {
				fmt.Printf("⚠️ No se pudo iniciar el servicio launchd: %v\n", err)
			} else {
				fmt.Println("✅ Demonio Pharus registrado e iniciado en el arranque de macOS.")
			}
		}

		// ----------------------------------------------------------------------
		// Guardar y Sincronizar
		// ----------------------------------------------------------------------
		if err := cfg.Save(); err != nil {
			fmt.Printf("❌ Error guardando configuración en %s: %v\n", cfg.ConfigFile, err)
			return
		}

		updateEnvFile(".env", cfg)

		fmt.Println("\n============================================================")
		fmt.Printf("✅ Configuración guardada correctamente en %s y .env\n", cfg.ConfigFile)
		fmt.Printf("📌 LLM Proveedor Activo: %s | Modelo: %s\n", strings.ToUpper(cfg.LLM.Provider), getActiveLLMModel(cfg))
		fmt.Printf("📌 Embeddings Provider:  %s | Modelo: %s | URL: %s\n", strings.ToUpper(cfg.Embed.Provider), cfg.Embed.Model, cfg.Embed.URL)
		fmt.Println("👉 Ejecuta 'pharus doctor' para verificar la salud completa de los servicios.")
	},
}

func isModelDownloadedLocally(repoID string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/tmp"
	}
	repoPathSanitized := strings.ReplaceAll(repoID, "/", "--")
	targetDir := filepath.Join(home, ".cache", "huggingface", "hub", "models--"+repoPathSanitized, "snapshots", "main")
	info, err := os.Stat(targetDir)
	if err == nil && info.IsDir() {
		entries, err := os.ReadDir(targetDir)
		if err == nil && len(entries) > 0 {
			return true
		}
	}
	return false
}

func validateEmbeddingServerImmediate(reader *bufio.Reader, cfg *config.Config) {
	fmt.Printf("\n🔍 Verificando estado del servidor de embeddings (%s en %s)...\n", strings.ToUpper(cfg.Embed.Provider), cfg.Embed.URL)
	provider := embedding.NewProvider(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	vec, err := provider.Embed(ctx, "test embedding connection")
	if err == nil && len(vec) > 0 {
		fmt.Printf("🟢 ¡Servidor %s Activo! Respondiendo correctamente con el modelo '%s' (%d dimensiones).\n", strings.ToUpper(cfg.Embed.Provider), cfg.Embed.Model, len(vec))
		return
	}

	fmt.Printf("⚠️ El servidor %s en %s no está respondiendo actualmente.\n", strings.ToUpper(cfg.Embed.Provider), cfg.Embed.URL)
	providerType := strings.ToLower(cfg.Embed.Provider)

	if providerType == "omlx" {
		fmt.Println("🔧 Instalando/iniciando OMLX como servicio local administrado...")
		if err := sysconfig.EnsureOMLX(context.Background(), cfg); err != nil {
			fmt.Printf("⚠️ No se pudo preparar OMLX automáticamente: %v\n", err)
		}
	} else if providerType == "ollama" {
		if _, err := exec.LookPath("ollama"); err == nil {
			fmt.Printf("👉 ¿Deseas autolanzar el servidor Ollama localmente ('ollama serve')? [S/n]: ")
			runAns, _ := reader.ReadString('\n')
			runAns = strings.ToLower(strings.TrimSpace(runAns))
			if runAns == "" || runAns == "s" || runAns == "y" || runAns == "si" {
				autostartOllamaServer(cfg)
			}
		}
	}
}

func autostartOMLXServer(cfg *config.Config) {
	if err := sysconfig.EnsureOMLX(context.Background(), cfg); err != nil {
		fmt.Printf("⚠️ No se pudo iniciar OMLX: %v\n", err)
	}
}

func autostartOllamaServer(cfg *config.Config) {
	fmt.Println("🚀 Autolanzando servidor Ollama ('ollama serve')...")
	cmd := exec.Command("ollama", "serve")
	if err := cmd.Start(); err != nil {
		fmt.Printf("⚠️ No se pudo iniciar 'ollama serve': %v\n", err)
		return
	}

	fmt.Print("⏳ Esperando que Ollama esté listo")
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		fmt.Print(".")
		resp, err := http.Get("http://localhost:11434/api/tags")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			fmt.Println("\n🟢 Servidor Ollama listo y respondiendo.")
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
	}
	fmt.Println("\nℹ️ Servidor Ollama lanzado en background.")
}

func downloadMLXModel(modelRepo string) {
	ctx := context.Background()
	if err := sysconfig.DownloadHFModelNative(ctx, modelRepo); err != nil {
		fmt.Printf("⚠️ No se pudo completar la descarga nativa de Hugging Face: %v\n", err)
	}
}

func getActiveLLMModel(cfg *config.Config) string {
	switch cfg.LLM.Provider {
	case "openai":
		return cfg.OpenAI.Model
	case "anthropic":
		return cfg.Anthropic.Model
	case "ollama":
		return cfg.OllamaLLM.Model
	case "llamacpp", "llama.cpp":
		return cfg.LlamaCPP.Model
	default:
		return cfg.MiniMax.Model
	}
}

func ensureDockerEngineRunning() error {
	if err := exec.Command("docker", "info").Run(); err == nil {
		return nil
	}

	fmt.Println("⚠️ El motor de Docker no responde. Verificando/Iniciando OrbStack o Docker Desktop...")

	if _, err := exec.LookPath("orbctl"); err == nil {
		fmt.Println("🚀 Ejecutando 'orbctl start' para inicializar el motor de Docker...")
		out, _ := exec.Command("orbctl", "start").CombinedOutput()
		outStr := strings.ToLower(string(out))
		if strings.Contains(outStr, "already running") || strings.Contains(outStr, "ready to use") {
			fmt.Println("ℹ️ OrbStack ya se encuentra activo en el sistema.")
		}
	} else {
		_ = exec.Command("open", "-a", "OrbStack").Run()
		_ = exec.Command("open", "-a", "Docker").Run()
	}

	fmt.Print("⏳ Esperando que el motor de Docker responda")
	for i := 0; i < 20; i++ {
		if err := exec.Command("docker", "info").Run(); err == nil {
			fmt.Println("\n✅ Motor de Docker listo y respondiendo correctamente.")
			return nil
		}
		time.Sleep(1 * time.Second)
		fmt.Print(".")
	}

	return fmt.Errorf("el motor de Docker no respondió a tiempo a la verificación de salud")
}

func deploySearXNGContainer() {
	if err := ensureDockerEngineRunning(); err != nil {
		fmt.Printf("⚠️ %v\n", err)
		fmt.Println("👉 Por favor inicia manualmente OrbStack o Docker Desktop e intenta de nuevo:")
		return
	}

	fmt.Println("🐳 Desplegando SearXNG administrado en Docker (puerto 8090)...")
	settingsPath, err := sysconfig.EnsureManagedSearXNG(context.Background(), cfg)
	if err != nil {
		if errors.Is(err, sysconfig.ErrSearXNGMigrationRequired) {
			fmt.Printf("⚠️ Migración explícita requerida: %v\n", err)
			return
		}
		fmt.Printf("⚠️ No se pudo desplegar SearXNG: %v\n", err)
	} else {
		fmt.Printf("✅ SearXNG administrado listo en http://localhost:8090 (configuración: %s)\n", settingsPath)
	}
}

func promptField(reader *bufio.Reader, label string, defaultValue string, isSecret bool) string {
	if defaultValue != "" {
		displayDefault := defaultValue
		if isSecret && len(defaultValue) > 8 {
			displayDefault = defaultValue[:4] + "..." + defaultValue[len(defaultValue)-4:]
		}
		fmt.Printf("🔹 %s [%s]: ", label, displayDefault)
	} else {
		fmt.Printf("🔹 %s: ", label)
	}

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue
	}
	return input
}

func updateEnvFile(envPath string, cfg *config.Config) {
	dataSTORMSources, _ := json.Marshal(cfg.DataSTORM.Sources)
	lines := []string{
		"# ==============================================================================",
		"# Pharus - Archivo de Configuración de Variables de Entorno (.env)",
		"# ==============================================================================",
		"",
		fmt.Sprintf("PHARUS_LLM_PROVIDER=\"%s\"", cfg.LLM.Provider),
		"",
		"# 1. MiniMax LLM Provider",
		fmt.Sprintf("PHARUS_MINIMAX_API_KEY=\"%s\"", cfg.MiniMax.APIKey),
		fmt.Sprintf("PHARUS_MINIMAX_GROUP_ID=\"%s\"", cfg.MiniMax.GroupID),
		fmt.Sprintf("PHARUS_MINIMAX_MODEL=\"%s\"", cfg.MiniMax.Model),
		fmt.Sprintf("PHARUS_MINIMAX_BASE_URL=\"%s\"", cfg.MiniMax.BaseURL),
		fmt.Sprintf("PHARUS_MINIMAX_MAX_TOKENS=\"%d\"", cfg.MiniMax.MaxTokens),
		"",
		"# 2. OpenAI LLM Provider",
		fmt.Sprintf("PHARUS_OPENAI_API_KEY=\"%s\"", cfg.OpenAI.APIKey),
		fmt.Sprintf("PHARUS_OPENAI_MODEL=\"%s\"", cfg.OpenAI.Model),
		fmt.Sprintf("PHARUS_OPENAI_BASE_URL=\"%s\"", cfg.OpenAI.BaseURL),
		"",
		"# 3. Anthropic LLM Provider",
		fmt.Sprintf("PHARUS_ANTHROPIC_API_KEY=\"%s\"", cfg.Anthropic.APIKey),
		fmt.Sprintf("PHARUS_ANTHROPIC_MODEL=\"%s\"", cfg.Anthropic.Model),
		fmt.Sprintf("PHARUS_ANTHROPIC_BASE_URL=\"%s\"", cfg.Anthropic.BaseURL),
		"",
		"# 4. Ollama Local LLM Provider",
		fmt.Sprintf("PHARUS_OLLAMA_LLM_URL=\"%s\"", cfg.OllamaLLM.BaseURL),
		fmt.Sprintf("PHARUS_OLLAMA_LLM_MODEL=\"%s\"", cfg.OllamaLLM.Model),
		"",
		"# 5. Embeddings Server (OMLX / MLX / Ollama / External)",
		fmt.Sprintf("PHARUS_EMBEDDING_PROVIDER=\"%s\"", cfg.Embed.Provider),
		fmt.Sprintf("PHARUS_EMBEDDING_URL=\"%s\"", cfg.Embed.URL),
		fmt.Sprintf("PHARUS_EMBEDDING_MODEL=\"%s\"", cfg.Embed.Model),
		fmt.Sprintf("PHARUS_EMBEDDING_API_KEY=\"%s\"", cfg.Embed.APIKey),
		"",
		"# 6. SearXNG Engine URL",
		fmt.Sprintf("PHARUS_SEARXNG_URL=\"%s\"", cfg.Search.SearXNGURL),
		"",
		"# 7. llama.cpp Local Inference Server (GBNF)",
		fmt.Sprintf("PHARUS_LLAMACPP_URL=\"%s\"", cfg.LlamaCPP.BaseURL),
		fmt.Sprintf("PHARUS_LLAMACPP_MODEL=\"%s\"", cfg.LlamaCPP.Model),
		fmt.Sprintf("PHARUS_LLAMACPP_GRAMMAR_ENABLE=\"%t\"", cfg.LlamaCPP.GrammarEnable),
		"",
		"# 8. DataSTORM CSV sources (YAML/JSON map: table: /absolute/file.csv)",
		fmt.Sprintf("PHARUS_DATASTORM_SOURCES='%s'", dataSTORMSources),
	}

	content := []byte(strings.Join(lines, "\n") + "\n")
	_ = os.WriteFile(envPath, content, 0600)

	home := config.GetHomeDir()
	pharusEnvPath := filepath.Join(home, ".pharus", ".env")
	_ = os.WriteFile(pharusEnvPath, content, 0600)
}

func init() {
	rootCmd.AddCommand(configCmd)
}
