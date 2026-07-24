package sysconfig

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/KyberixCo/Pharus/internal/config"
)

const (
	managedSearXNGContainer = "pharus-searxng"
	managedSearXNGImage     = "searxng/searxng:2026.7.19-6da6eee26"
	managedSearXNGHostPort  = "8090"
)

var ErrSearXNGMigrationRequired = errors.New("existing SearXNG container requires explicit migration")

type commandRunner func(context.Context, string, ...string) ([]byte, error)

// EnsureManagedSearXNG writes the managed settings and creates a pinned,
// repeatable Docker deployment. It never replaces an existing incompatible
// container: callers must migrate it explicitly.
func EnsureManagedSearXNG(ctx context.Context, cfg *config.Config) (string, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return "", fmt.Errorf("Docker CLI is not installed: %w", err)
	}
	return ensureManagedSearXNG(ctx, cfg, runCommand)
}

func ensureManagedSearXNG(ctx context.Context, cfg *config.Config, run commandRunner) (string, error) {
	if _, err := run(ctx, "docker", "info"); err != nil {
		return "", fmt.Errorf("Docker engine is unavailable: %w", err)
	}

	settingsPath, err := writeManagedSearXNGSettings(cfg.BaseDir)
	if err != nil {
		return "", err
	}
	image := cfg.Search.SearXNGImage
	if image == "" {
		image = managedSearXNGImage
	}
	inspection, err := run(ctx, "docker", "inspect", "--format", "{{.Config.Image}}|{{range .Mounts}}{{.Source}}:{{.Destination}};{{end}}", managedSearXNGContainer)
	if err == nil {
		if !isManagedContainer(string(inspection), image, settingsPath) {
			return "", fmt.Errorf("%w: container %q is not managed by this Pharus configuration; review it and recreate it with the managed settings at %s", ErrSearXNGMigrationRequired, managedSearXNGContainer, settingsPath)
		}
		return settingsPath, nil
	}

	_, err = run(ctx, "docker", "run", "-d", "-p", managedSearXNGHostPort+":8080", "--name", managedSearXNGContainer,
		"--restart=always", "-v", settingsPath+":/etc/searxng/settings.yml:ro", image)
	if err != nil {
		return "", fmt.Errorf("create managed SearXNG container: %w", err)
	}
	return settingsPath, nil
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func isManagedContainer(inspection, image, settingsPath string) bool {
	parts := strings.SplitN(strings.TrimSpace(inspection), "|", 2)
	return len(parts) == 2 && parts[0] == image && strings.Contains(parts[1], settingsPath+":/etc/searxng/settings.yml")
}

func writeManagedSearXNGSettings(baseDir string) (string, error) {
	dir := filepath.Join(baseDir, "searxng")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create SearXNG configuration directory: %w", err)
	}
	path := filepath.Join(dir, "settings.yml")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("generate SearXNG secret key: %w", err)
	}
	settings := fmt.Sprintf("use_default_settings: true\n\nserver:\n  secret_key: %s\n\nsearch:\n  formats:\n    - html\n    - json\n", hex.EncodeToString(secret))
	if err := os.WriteFile(path, []byte(settings), 0600); err != nil {
		return "", fmt.Errorf("write managed SearXNG settings: %w", err)
	}
	return path, nil
}
