package sysconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/KyberixCo/Pharus/internal/config"
	"github.com/KyberixCo/Pharus/internal/scraper"
)

type DoctorCheck struct {
	Name    string
	Passed  bool
	Details string
}

func RunDiagnostics(ctx context.Context, cfg *config.Config) []DoctorCheck {
	var checks []DoctorCheck

	// 1. Config Check
	if _, err := os.Stat(cfg.ConfigFile); err == nil {
		checks = append(checks, DoctorCheck{
			Name:    "Config File",
			Passed:  true,
			Details: fmt.Sprintf("Found at %s", cfg.ConfigFile),
		})
	} else {
		checks = append(checks, DoctorCheck{
			Name:    "Config File",
			Passed:  false,
			Details: "Config file missing. Run 'pharus setup'",
		})
	}

	// 2. Active LLM Provider Check
	provider := strings.ToLower(cfg.LLM.Provider)
	if provider == "" {
		provider = "minimax"
	}

	switch provider {
	case "openai":
		if cfg.OpenAI.APIKey != "" {
			checks = append(checks, DoctorCheck{
				Name:    "LLM Provider (OpenAI)",
				Passed:  true,
				Details: fmt.Sprintf("Model %s configured (%s)", cfg.OpenAI.Model, maskKey(cfg.OpenAI.APIKey)),
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:    "LLM Provider (OpenAI)",
				Passed:  false,
				Details: "Missing API Key. Set PHARUS_OPENAI_API_KEY env or configure in config.yaml",
			})
		}
	case "anthropic":
		if cfg.Anthropic.APIKey != "" {
			checks = append(checks, DoctorCheck{
				Name:    "LLM Provider (Anthropic)",
				Passed:  true,
				Details: fmt.Sprintf("Model %s configured (%s)", cfg.Anthropic.Model, maskKey(cfg.Anthropic.APIKey)),
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:    "LLM Provider (Anthropic)",
				Passed:  false,
				Details: "Missing API Key. Set PHARUS_ANTHROPIC_API_KEY env or configure in config.yaml",
			})
		}
	case "ollama":
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get(cfg.OllamaLLM.BaseURL + "/api/tags")
		if err == nil && (resp.StatusCode == 200 || resp.StatusCode == 404) {
			resp.Body.Close()
			checks = append(checks, DoctorCheck{
				Name:    "LLM Provider (Local Ollama)",
				Passed:  true,
				Details: fmt.Sprintf("Model %s reachable at %s", cfg.OllamaLLM.Model, cfg.OllamaLLM.BaseURL),
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:    "LLM Provider (Local Ollama)",
				Passed:  false,
				Details: fmt.Sprintf("Unreachable at %s. Ensure ollama is running ('ollama serve')", cfg.OllamaLLM.BaseURL),
			})
		}
	case "llamacpp", "llama.cpp":
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get(strings.TrimRight(cfg.LlamaCPP.BaseURL, "/") + "/health")
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			resp.Body.Close()
			checks = append(checks, DoctorCheck{
				Name:    "LLM Provider (llama.cpp)",
				Passed:  true,
				Details: fmt.Sprintf("Model %s reachable at %s", cfg.LlamaCPP.Model, cfg.LlamaCPP.BaseURL),
			})
		} else {
			if resp != nil {
				resp.Body.Close()
			}
			checks = append(checks, DoctorCheck{
				Name:    "LLM Provider (llama.cpp)",
				Passed:  false,
				Details: fmt.Sprintf("Unreachable at %s. Start llama-server and verify /health", cfg.LlamaCPP.BaseURL),
			})
		}
	default:
		if cfg.MiniMax.APIKey != "" {
			checks = append(checks, DoctorCheck{
				Name:    "LLM Provider (MiniMax)",
				Passed:  true,
				Details: fmt.Sprintf("Model %s configured (%s)", cfg.MiniMax.Model, maskKey(cfg.MiniMax.APIKey)),
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:    "LLM Provider (MiniMax)",
				Passed:  false,
				Details: "Missing API Key. Set PHARUS_MINIMAX_API_KEY env or configure in config.yaml",
			})
		}
	}

	// 3. Active Embeddings Endpoint Check (MLX / Ollama / External API)
	client := &http.Client{Timeout: 3 * time.Second}
	embedProvider := strings.ToLower(cfg.Embed.Provider)
	if embedProvider == "" {
		embedProvider = "mlx"
	}

	switch embedProvider {
	case "omlx":
		resp, err := client.Get(cfg.Embed.URL + "/v1/models")
		if err == nil && resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			var models struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&models)
			resp.Body.Close()
			available := make([]string, 0, len(models.Data))
			modelFound := false
			for _, model := range models.Data {
				available = append(available, model.ID)
				if omlxModelIDMatches(cfg.Embed.Model, model.ID) {
					modelFound = true
				}
			}
			if decodeErr != nil || !modelFound {
				checks = append(checks, DoctorCheck{
					Name:    "Embedding Server (OMLX - Open MLX)",
					Passed:  false,
					Details: fmt.Sprintf("Configured model %s is unavailable at %s (available: %s)", cfg.Embed.Model, cfg.Embed.URL, strings.Join(available, ", ")),
				})
				break
			}
			checks = append(checks, DoctorCheck{
				Name:    "Embedding Server (OMLX - Open MLX)",
				Passed:  true,
				Details: fmt.Sprintf("Reachable at %s (model: %s)", cfg.Embed.URL, cfg.Embed.Model),
			})
		} else {
			if resp != nil {
				resp.Body.Close()
			}
			checks = append(checks, DoctorCheck{
				Name:    "Embedding Server (OMLX - Open MLX)",
				Passed:  false,
				Details: fmt.Sprintf("Unreachable at %s. (Fallback determinista activo)", cfg.Embed.URL),
			})
		}
	case "mlx":
		resp, err := client.Get(cfg.Embed.URL + "/v1/models")
		if err == nil && resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			resp.Body.Close()
			checks = append(checks, DoctorCheck{
				Name:    "Embedding Server (MLX Framework)",
				Passed:  true,
				Details: fmt.Sprintf("Reachable at %s (model: %s)", cfg.Embed.URL, cfg.Embed.Model),
			})
		} else {
			if resp != nil {
				resp.Body.Close()
			}
			checks = append(checks, DoctorCheck{
				Name:    "Embedding Server (MLX Framework)",
				Passed:  false,
				Details: fmt.Sprintf("Unreachable at %s. (Fallback determinista activo)", cfg.Embed.URL),
			})
		}
	case "openai", "external", "custom":
		checks = append(checks, DoctorCheck{
			Name:    "Embedding Server (External/OpenAI)",
			Passed:  true,
			Details: fmt.Sprintf("Model %s configured at %s", cfg.Embed.Model, cfg.Embed.URL),
		})
	default: // "ollama"
		resp, err := client.Get(cfg.Embed.URL + "/api/tags")
		if err == nil && (resp.StatusCode == 200 || resp.StatusCode == 404) {
			resp.Body.Close()
			checks = append(checks, DoctorCheck{
				Name:    "Embedding Server (Ollama)",
				Passed:  true,
				Details: fmt.Sprintf("Reachable at %s (model: %s)", cfg.Embed.URL, cfg.Embed.Model),
			})
		} else {
			if resp != nil {
				resp.Body.Close()
			}
			checks = append(checks, DoctorCheck{
				Name:    "Embedding Server (Ollama)",
				Passed:  false,
				Details: fmt.Sprintf("Unreachable at %s. (Fallback determinista activo)", cfg.Embed.URL),
			})
		}
	}

	// 4. SearXNG capability check. A reachable HTML endpoint is insufficient:
	// research requires /search?format=json to return a decodable results array.
	searchClient, err := scraper.NewSearXNGClientWithOptions(
		cfg.Search.SearXNGURL,
		time.Duration(cfg.Search.SearXNGTimeoutSeconds)*time.Second,
		cfg.Search.SearXNGMaxResponseBytes,
	)
	if err == nil {
		err = searchClient.CheckJSONCapability(ctx)
	}
	if err == nil {
		checks = append(checks, DoctorCheck{
			Name:    "SearXNG JSON Search",
			Passed:  true,
			Details: fmt.Sprintf("JSON search capability verified at %s", redactURL(cfg.Search.SearXNGURL)),
		})
	} else {
		checks = append(checks, DoctorCheck{
			Name:    "SearXNG JSON Search",
			Passed:  false,
			Details: fmt.Sprintf("JSON search unavailable at %s: %v", redactURL(cfg.Search.SearXNGURL), err),
		})
	}

	// 5. Unix Domain Socket Check
	if _, err := os.Stat(cfg.SocketPath); err == nil {
		conn, err := net.DialTimeout("unix", cfg.SocketPath, 2*time.Second)
		if err == nil {
			conn.Close()
			checks = append(checks, DoctorCheck{
				Name:    "Daemon Process (UDS Socket)",
				Passed:  true,
				Details: fmt.Sprintf("Active on %s", cfg.SocketPath),
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:    "Daemon Process (UDS Socket)",
				Passed:  false,
				Details: fmt.Sprintf("Socket file exists but connection failed: %v", err),
			})
		}
	} else {
		checks = append(checks, DoctorCheck{
			Name:    "Daemon Process (UDS Socket)",
			Passed:  false,
			Details: "Daemon is not running. Start with 'pharus daemon start'",
		})
	}

	return checks
}

func omlxModelIDMatches(configured, available string) bool {
	candidates := []string{configured, strings.ReplaceAll(configured, "/", "--")}
	if slash := strings.Index(configured, "/"); slash >= 0 && slash+1 < len(configured) {
		candidates = append(candidates, configured[slash+1:])
	}
	for _, candidate := range candidates {
		if strings.EqualFold(candidate, available) {
			return true
		}
	}
	return false
}

func maskKey(k string) string {
	if len(k) <= 8 {
		return "****"
	}
	return k[:4] + "..." + k[len(k)-4:]
}

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "configured endpoint"
	}
	u.User = nil
	return u.String()
}
