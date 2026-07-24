package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Language    string `yaml:"language"`
	BaseDir     string `yaml:"base_dir"`
	ConfigFile  string `yaml:"config_file"`
	SocketPath  string `yaml:"socket_path"`
	VectorDir   string `yaml:"vector_dir"`
	LogsDir     string `yaml:"logs_dir"`
	HTTPHost    string `yaml:"http_host"`
	HTTPPort    int    `yaml:"http_port"`
	DaemonToken string `yaml:"daemon_token"`
	LogLevel    string `yaml:"log_level"`

	LLM       LLMConfig       `yaml:"llm"`
	MiniMax   MiniMaxConfig   `yaml:"minimax"`
	OpenAI    OpenAIConfig    `yaml:"openai"`
	Anthropic AnthropicConfig `yaml:"anthropic"`
	OllamaLLM OllamaLLMConfig `yaml:"ollama_llm"`
	LlamaCPP  LlamaCPPConfig  `yaml:"llamacpp"`
	Embed     EmbedConfig     `yaml:"embed"`
	Vector    VectorConfig    `yaml:"vector"`
	Search    SearchConfig    `yaml:"search"`
	Research  ResearchConfig  `yaml:"research"`
	DataSTORM DataSTORMConfig `yaml:"datastorm"`
	Sandbox   SandboxConfig   `yaml:"sandbox"`
	Security  SecurityConfig  `yaml:"security"`
}

type LLMConfig struct {
	Provider                   string `yaml:"provider"` // "minimax", "openai", "anthropic", "ollama", "llamacpp"
	RetryMaxAttempts           int    `yaml:"retry_max_attempts"`
	RetryInitialBackoffSeconds int    `yaml:"retry_initial_backoff_seconds"`
	RetryMaxBackoffSeconds     int    `yaml:"retry_max_backoff_seconds"`
}

type OpenAIConfig struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"` // default: "https://api.openai.com/v1"
	Model   string `yaml:"model"`    // default: "gpt-4o-mini"
}

type AnthropicConfig struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"` // default: "https://api.anthropic.com/v1"
	Model   string `yaml:"model"`    // default: "claude-3-5-sonnet-20241022"
}

type OllamaLLMConfig struct {
	BaseURL string `yaml:"base_url"` // default: "http://localhost:11434"
	Model   string `yaml:"model"`    // default: "llama3.1"
}

type SecurityConfig struct {
	MaxRequestBodyMB int `yaml:"max_request_body_mb"` // default: 5
	MaxSSEBufferMB   int `yaml:"max_sse_buffer_mb"`   // default: 10
}

type LlamaCPPConfig struct {
	BaseURL       string  `yaml:"base_url"`       // e.g., "http://localhost:8080"
	Model         string  `yaml:"model"`          // e.g., "qwen2.5-coder-7b-instruct"
	GrammarEnable bool    `yaml:"grammar_enable"` // default: true
	Temperature   float64 `yaml:"temperature"`    // default: 0.2
	TopK          int     `yaml:"top_k"`          // default: 40
	TopP          float64 `yaml:"top_p"`          // default: 0.95
}

type SandboxConfig struct {
	Engine       string  `yaml:"engine"`        // "native", "docker", "auto"
	MemoryMB     int     `yaml:"memory_mb"`     // default: 512
	CPUScores    float64 `yaml:"cpu_scores"`    // default: 1.0
	AllowNetwork bool    `yaml:"allow_network"` // default: false
	WorkDir      string  `yaml:"work_dir"`      // default: "/tmp/pharus_sandbox"
}

type MiniMaxConfig struct {
	APIKey    string `yaml:"api_key"`
	GroupID   string `yaml:"group_id"`
	BaseURL   string `yaml:"base_url"`
	Model     string `yaml:"model"`
	MaxTokens int    `yaml:"max_tokens"`
}

type EmbedConfig struct {
	Provider string `yaml:"provider"` // "mlx", "ollama", "openai", "custom"
	URL      string `yaml:"url"`      // e.g., "http://localhost:8000" (mlx), "http://localhost:11434" (ollama)
	Model    string `yaml:"model"`    // e.g., "bge-small-en-v1.5", "nomic-embed-text"
	APIKey   string `yaml:"api_key"`  // API Key para proveedores externos
}

// VectorConfig selects storage independently of the embedding provider.
// Collection is retained for non-research callers; research runs are isolated
// with a research_id metadata filter.
type VectorConfig struct {
	Provider   string `yaml:"provider"` // "chromem" or "lancedb"
	Collection string `yaml:"collection"`
}

type SearchConfig struct {
	SearXNGURL              string `yaml:"searxng_url"`
	SearXNGTimeoutSeconds   int    `yaml:"searxng_timeout_seconds"`
	SearXNGMaxResponseBytes int64  `yaml:"searxng_max_response_bytes"`
	SearXNGImage            string `yaml:"searxng_image"`
	QueryMaxCharacters      int    `yaml:"query_max_characters"`
	QueryMaxTerms           int    `yaml:"query_max_terms"`
	EvidenceMinimumUsable   int    `yaml:"evidence_minimum_usable"`
	EvidenceMinimumURLs     int    `yaml:"evidence_minimum_urls"`
	EvidenceMinimumFullText int    `yaml:"evidence_minimum_full_text"`
}

type ResearchConfig struct {
	ResumeSessions        bool   `yaml:"resume_sessions"`
	CheckpointMaxAgeHours int    `yaml:"checkpoint_max_age_hours"`
	DefaultProfile        string `yaml:"default_profile"`
}

// DataSTORMConfig maps SQL table names to CSV files loaded by the standard
// CLI/daemon engine constructors.
type DataSTORMConfig struct {
	Sources map[string]string `yaml:"sources"`
}

// GetHomeDir returns the user's home directory.
func GetHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/tmp"
	}
	return home
}

// DefaultConfig returns sensible defaults for Pharus.
func DefaultConfig() *Config {
	home := GetHomeDir()
	baseDir := filepath.Join(home, ".pharus")

	return &Config{
		Language:    "auto",
		BaseDir:     baseDir,
		ConfigFile:  filepath.Join(baseDir, "config.yaml"),
		SocketPath:  filepath.Join(baseDir, "pharus.sock"),
		VectorDir:   filepath.Join(baseDir, "vectors"),
		LogsDir:     filepath.Join(baseDir, "logs"),
		HTTPHost:    "127.0.0.1",
		HTTPPort:    8765,
		DaemonToken: generateToken(),
		LogLevel:    "info",
		LLM: LLMConfig{
			Provider:                   "minimax",
			RetryMaxAttempts:           5,
			RetryInitialBackoffSeconds: 2,
			RetryMaxBackoffSeconds:     30,
		},
		MiniMax: MiniMaxConfig{
			APIKey:    os.Getenv("PHARUS_MINIMAX_API_KEY"),
			GroupID:   os.Getenv("PHARUS_MINIMAX_GROUP_ID"),
			BaseURL:   "https://api.minimax.io/anthropic",
			Model:     "MiniMax-M3",
			MaxTokens: 16384,
		},
		OpenAI: OpenAIConfig{
			APIKey:  os.Getenv("PHARUS_OPENAI_API_KEY"),
			BaseURL: "https://api.openai.com/v1",
			Model:   "gpt-4o-mini",
		},
		Anthropic: AnthropicConfig{
			APIKey:  os.Getenv("PHARUS_ANTHROPIC_API_KEY"),
			BaseURL: "https://api.anthropic.com/v1",
			Model:   "claude-3-5-sonnet-20241022",
		},
		OllamaLLM: OllamaLLMConfig{
			BaseURL: "http://localhost:11434",
			Model:   "llama3.1",
		},
		LlamaCPP: LlamaCPPConfig{
			BaseURL:       "http://localhost:8080",
			Model:         "qwen2.5-coder-7b-instruct",
			GrammarEnable: true,
			Temperature:   0.2,
			TopK:          40,
			TopP:          0.95,
		},
		Embed: EmbedConfig{
			Provider: "omlx",
			URL:      "http://localhost:8000",
			Model:    "mlx-community/Qwen3-Embedding-0.6B-8bit",
			APIKey:   os.Getenv("PHARUS_EMBEDDING_API_KEY"),
		},
		Vector: VectorConfig{Provider: "chromem", Collection: "pharus_research"},
		Search: SearchConfig{
			SearXNGURL:              "http://localhost:8090",
			SearXNGTimeoutSeconds:   10,
			SearXNGMaxResponseBytes: 2 << 20,
			SearXNGImage:            "searxng/searxng:2026.7.19-6da6eee26",
			QueryMaxCharacters:      160,
			QueryMaxTerms:           20,
			EvidenceMinimumUsable:   3,
			EvidenceMinimumURLs:     2,
			EvidenceMinimumFullText: 1,
		},
		Research: ResearchConfig{
			ResumeSessions:        true,
			CheckpointMaxAgeHours: 72,
			DefaultProfile:        "balanced",
		},
		DataSTORM: DataSTORMConfig{Sources: map[string]string{}},
		Sandbox: SandboxConfig{
			Engine:       "auto",
			MemoryMB:     512,
			CPUScores:    1.0,
			AllowNetwork: false,
			WorkDir:      filepath.Join(os.TempDir(), "pharus_sandbox"),
		},
		Security: SecurityConfig{
			MaxRequestBodyMB: 5,
			MaxSSEBufferMB:   10,
		},
	}
}

// LoadConfig loads the configuration from file or creates a default one.
func LoadConfig() (*Config, error) {
	// 1. Intentar cargar .env desde múltiples ubicaciones conocidas
	loadEnvFile(".env")
	if exePath, err := os.Executable(); err == nil {
		loadEnvFile(filepath.Join(filepath.Dir(exePath), ".env"))
		loadEnvFile(filepath.Join(filepath.Dir(exePath), "..", ".env"))
	}
	home := GetHomeDir()
	loadEnvFile(filepath.Join(home, ".pharus", ".env"))

	cfg := DefaultConfig()

	if err := os.MkdirAll(cfg.BaseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base dir %s: %w", cfg.BaseDir, err)
	}
	_ = os.MkdirAll(cfg.VectorDir, 0755)
	_ = os.MkdirAll(cfg.LogsDir, 0755)

	// 2. Leer archivo config.yaml si existe
	if _, err := os.Stat(cfg.ConfigFile); err == nil {
		data, err := os.ReadFile(cfg.ConfigFile)
		if err == nil {
			_ = yaml.Unmarshal(data, cfg)
		}
	}

	// 3. Variables de entorno (máxima prioridad)
	if envLanguage := os.Getenv("PHARUS_LANGUAGE"); envLanguage != "" {
		cfg.Language = envLanguage
	}
	if envProvider := os.Getenv("PHARUS_LLM_PROVIDER"); envProvider != "" {
		cfg.LLM.Provider = envProvider
	}
	if value := positiveEnvInt("PHARUS_LLM_RETRY_MAX_ATTEMPTS"); value > 0 {
		cfg.LLM.RetryMaxAttempts = value
	}
	if value := positiveEnvInt("PHARUS_LLM_RETRY_INITIAL_BACKOFF_SECONDS"); value > 0 {
		cfg.LLM.RetryInitialBackoffSeconds = value
	}
	if value := positiveEnvInt("PHARUS_LLM_RETRY_MAX_BACKOFF_SECONDS"); value > 0 {
		cfg.LLM.RetryMaxBackoffSeconds = value
	}
	if envKey := os.Getenv("PHARUS_MINIMAX_API_KEY"); envKey != "" {
		cfg.MiniMax.APIKey = envKey
	}
	if envGroup := os.Getenv("PHARUS_MINIMAX_GROUP_ID"); envGroup != "" {
		cfg.MiniMax.GroupID = envGroup
	}
	if envMiniMaxModel := os.Getenv("PHARUS_MINIMAX_MODEL"); envMiniMaxModel != "" {
		cfg.MiniMax.Model = envMiniMaxModel
	}
	if envMiniMaxBase := os.Getenv("PHARUS_MINIMAX_BASE_URL"); envMiniMaxBase != "" {
		cfg.MiniMax.BaseURL = envMiniMaxBase
	}
	if envMiniMaxTokens := os.Getenv("PHARUS_MINIMAX_MAX_TOKENS"); envMiniMaxTokens != "" {
		var maxTokens int
		if _, err := fmt.Sscanf(envMiniMaxTokens, "%d", &maxTokens); err == nil && maxTokens > 0 {
			cfg.MiniMax.MaxTokens = maxTokens
		}
	}
	if envOpenAIKey := os.Getenv("PHARUS_OPENAI_API_KEY"); envOpenAIKey != "" {
		cfg.OpenAI.APIKey = envOpenAIKey
	}
	if envOpenAIModel := os.Getenv("PHARUS_OPENAI_MODEL"); envOpenAIModel != "" {
		cfg.OpenAI.Model = envOpenAIModel
	}
	if envOpenAIBase := os.Getenv("PHARUS_OPENAI_BASE_URL"); envOpenAIBase != "" {
		cfg.OpenAI.BaseURL = envOpenAIBase
	}
	if envAnthropicKey := os.Getenv("PHARUS_ANTHROPIC_API_KEY"); envAnthropicKey != "" {
		cfg.Anthropic.APIKey = envAnthropicKey
	}
	if envAnthropicModel := os.Getenv("PHARUS_ANTHROPIC_MODEL"); envAnthropicModel != "" {
		cfg.Anthropic.Model = envAnthropicModel
	}
	if envAnthropicBase := os.Getenv("PHARUS_ANTHROPIC_BASE_URL"); envAnthropicBase != "" {
		cfg.Anthropic.BaseURL = envAnthropicBase
	}
	if envOllamaURL := os.Getenv("PHARUS_OLLAMA_LLM_URL"); envOllamaURL != "" {
		cfg.OllamaLLM.BaseURL = envOllamaURL
	}
	if envOllamaModel := os.Getenv("PHARUS_OLLAMA_LLM_MODEL"); envOllamaModel != "" {
		cfg.OllamaLLM.Model = envOllamaModel
	}
	if envSearx := os.Getenv("PHARUS_SEARXNG_URL"); envSearx != "" {
		cfg.Search.SearXNGURL = envSearx
	}
	if envSearxImage := os.Getenv("PHARUS_SEARXNG_IMAGE"); envSearxImage != "" {
		cfg.Search.SearXNGImage = envSearxImage
	}
	if envEmbedProvider := os.Getenv("PHARUS_EMBEDDING_PROVIDER"); envEmbedProvider != "" {
		cfg.Embed.Provider = envEmbedProvider
	}
	if envEmbed := os.Getenv("PHARUS_EMBEDDING_URL"); envEmbed != "" {
		cfg.Embed.URL = envEmbed
	}
	if envEmbedModel := os.Getenv("PHARUS_EMBEDDING_MODEL"); envEmbedModel != "" {
		cfg.Embed.Model = envEmbedModel
	}
	if envEmbedKey := os.Getenv("PHARUS_EMBEDDING_API_KEY"); envEmbedKey != "" {
		cfg.Embed.APIKey = envEmbedKey
	}
	if envVectorProvider := os.Getenv("PHARUS_VECTOR_PROVIDER"); envVectorProvider != "" {
		cfg.Vector.Provider = envVectorProvider
	}
	if envVectorCollection := os.Getenv("PHARUS_VECTOR_COLLECTION"); envVectorCollection != "" {
		cfg.Vector.Collection = envVectorCollection
	}
	if value, ok := envBool("PHARUS_RESEARCH_RESUME_SESSIONS"); ok {
		cfg.Research.ResumeSessions = value
	}
	if value := positiveEnvInt("PHARUS_RESEARCH_CHECKPOINT_MAX_AGE_HOURS"); value > 0 {
		cfg.Research.CheckpointMaxAgeHours = value
	}
	if value := os.Getenv("PHARUS_RESEARCH_PROFILE"); value != "" {
		cfg.Research.DefaultProfile = value
	}
	if envLlama := os.Getenv("PHARUS_LLAMACPP_URL"); envLlama != "" {
		cfg.LlamaCPP.BaseURL = envLlama
	}
	if envLlamaModel := os.Getenv("PHARUS_LLAMACPP_MODEL"); envLlamaModel != "" {
		cfg.LlamaCPP.Model = envLlamaModel
	}
	if enabled, ok := envBool("PHARUS_LLAMACPP_GRAMMAR_ENABLE"); ok {
		cfg.LlamaCPP.GrammarEnable = enabled
	}
	if envSources := os.Getenv("PHARUS_DATASTORM_SOURCES"); envSources != "" {
		var sources map[string]string
		if err := yaml.Unmarshal([]byte(envSources), &sources); err == nil {
			cfg.DataSTORM.Sources = sources
		}
	}

	// 4. Guardar configuración actualizada en config.yaml para persistencia global
	_ = cfg.Save()

	return cfg, nil
}

// Save writes the current configuration to disk.
func (c *Config) Save() error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(c.ConfigFile, data, 0600)
}

func generateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func positiveEnvInt(name string) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0
	}
	var value int
	if _, err := fmt.Sscanf(raw, "%d", &value); err != nil || value <= 0 {
		return 0
	}
	return value
}

func envBool(name string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func loadEnvFile(envPath string) {
	data, err := os.ReadFile(envPath)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			val = strings.Trim(val, "\"'")
			if val != "" && os.Getenv(key) == "" {
				os.Setenv(key, val)
			}
		}
	}
}
