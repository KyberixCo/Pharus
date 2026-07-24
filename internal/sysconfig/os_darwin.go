package sysconfig

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/KyberixCo/Pharus/internal/config"
)

type SetupResult struct {
	Component string
	Success   bool
	Message   string
}

// AutoConfigureOS performs automated system setup for macOS.
func AutoConfigureOS(ctx context.Context, cfg *config.Config) ([]SetupResult, error) {
	var results []SetupResult
	brew := NewBrewManager()

	// 1. Check OS
	if runtime.GOOS != "darwin" {
		results = append(results, SetupResult{
			Component: "OS Check",
			Success:   false,
			Message:   fmt.Sprintf("Warning: Pharus is optimized for macOS (running on %s)", runtime.GOOS),
		})
	} else {
		results = append(results, SetupResult{
			Component: "OS Check",
			Success:   true,
			Message:   fmt.Sprintf("macOS (%s) detected", runtime.GOARCH),
		})
	}

	// 2. Ensure base directories exist
	dirs := []string{cfg.BaseDir, cfg.VectorDir, cfg.LogsDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			results = append(results, SetupResult{
				Component: "Directories",
				Success:   false,
				Message:   fmt.Sprintf("Failed to create dir %s: %v", dir, err),
			})
		} else {
			results = append(results, SetupResult{
				Component: "Directories",
				Success:   true,
				Message:   fmt.Sprintf("Directory ready: %s", dir),
			})
		}
	}

	// 3. Save initial config if missing
	if err := cfg.Save(); err != nil {
		results = append(results, SetupResult{
			Component: "Configuration",
			Success:   false,
			Message:   fmt.Sprintf("Failed to write config: %v", err),
		})
	} else {
		results = append(results, SetupResult{
			Component: "Configuration",
			Success:   true,
			Message:   fmt.Sprintf("Config saved at %s", cfg.ConfigFile),
		})
	}

	// 4. Provision managed SearXNG when Docker is available. A failed optional
	// provisioning is reported explicitly; it does not prevent the rest of setup.
	if _, err := exec.LookPath("docker"); err != nil {
		results = append(results, SetupResult{
			Component: "SearXNG JSON Search",
			Success:   false,
			Message:   "Docker is not installed; configure an external SearXNG instance or install Docker.",
		})
	} else if settingsPath, err := EnsureManagedSearXNG(ctx, cfg); err != nil {
		results = append(results, SetupResult{
			Component: "SearXNG JSON Search",
			Success:   false,
			Message:   fmt.Sprintf("Managed SearXNG was not provisioned: %v", err),
		})
	} else {
		results = append(results, SetupResult{
			Component: "SearXNG JSON Search",
			Success:   true,
			Message:   fmt.Sprintf("Managed SearXNG configured with %s", settingsPath),
		})
	}

	// 5. Check Homebrew
	if !brew.IsBrewInstalled(ctx) {
		results = append(results, SetupResult{
			Component: "Homebrew",
			Success:   false,
			Message:   "Homebrew is not installed. Please install brew from https://brew.sh",
		})
	} else {
		results = append(results, SetupResult{
			Component: "Homebrew",
			Success:   true,
			Message:   "Homebrew is installed and ready",
		})
	}

	// 5. Check Ollama (Local Embeddings)
	ollamaInstalled, _ := brew.IsFormulaInstalled(ctx, "ollama")
	if !ollamaInstalled {
		// Try looking in PATH
		_, err := exec.LookPath("ollama")
		if err == nil {
			ollamaInstalled = true
		}
	}

	if ollamaInstalled {
		results = append(results, SetupResult{
			Component: "Ollama Embeddings",
			Success:   true,
			Message:   "Ollama is installed locally",
		})
	} else {
		results = append(results, SetupResult{
			Component: "Ollama Embeddings",
			Success:   false,
			Message:   "Ollama is not installed. You can install it via 'brew install ollama' for local embeddings.",
		})
	}

	return results, nil
}
