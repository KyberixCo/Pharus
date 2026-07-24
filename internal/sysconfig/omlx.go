package sysconfig

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/KyberixCo/Pharus/internal/config"
)

const omlxTap = "jundot/omlx"

// EnsureOMLX installs (when needed) and starts oMLX as a managed Homebrew
// service. It deliberately only manages a local endpoint: a remote embedding
// service is owned by its operator and must never be modified by Pharus.
func EnsureOMLX(ctx context.Context, cfg *config.Config) error {
	if !isLocalOMLXURL(cfg.Embed.URL) {
		return fmt.Errorf("la URL de OMLX debe ser local para administrarla automáticamente: %s", cfg.Embed.URL)
	}

	if omlxReachable(ctx, cfg.Embed.URL) {
		return nil
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return fmt.Errorf("OMLX requiere macOS con Apple Silicon; configura otro proveedor de embeddings")
	}

	if _, err := exec.LookPath("brew"); err != nil {
		return fmt.Errorf("Homebrew no está instalado; instala Homebrew para que Pharus pueda instalar OMLX")
	}
	if _, err := exec.LookPath("omlx"); err != nil {
		if output, err := exec.CommandContext(ctx, "brew", "tap", omlxTap, "https://github.com/jundot/omlx").CombinedOutput(); err != nil {
			return fmt.Errorf("no se pudo añadir el tap de OMLX: %s: %w", strings.TrimSpace(string(output)), err)
		}
		if output, err := exec.CommandContext(ctx, "brew", "install", "omlx").CombinedOutput(); err != nil {
			return fmt.Errorf("no se pudo instalar OMLX: %s: %w", strings.TrimSpace(string(output)), err)
		}
	}

	if output, err := exec.CommandContext(ctx, "brew", "services", "start", "omlx").CombinedOutput(); err != nil {
		return fmt.Errorf("no se pudo iniciar el servicio OMLX: %s: %w", strings.TrimSpace(string(output)), err)
	}

	deadline := time.NewTimer(45 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if omlxReachable(ctx, cfg.Embed.URL) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("OMLX se inició, pero no respondió en %s tras 45 segundos", cfg.Embed.URL)
		case <-ticker.C:
		}
	}
}

func isLocalOMLXURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return false
	}
	if port := parsed.Port(); port != "" && port != "8000" {
		return false
	}
	return net.ParseIP(host) != nil || host == "localhost"
}

func omlxReachable(ctx context.Context, baseURL string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/v1/models", nil)
	if err != nil {
		return false
	}
	response, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices
}
