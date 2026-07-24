//go:build darwin

package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/KyberixCo/Pharus/internal/config"
)

func nativeSandboxAvailable(cfg config.SandboxConfig) error {
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		return fmt.Errorf("native sandbox requires sandbox-exec: %w", err)
	}
	if cfg.MemoryMB > 0 {
		return fmt.Errorf("native sandbox cannot enforce a per-process memory limit on macOS; set sandbox.engine to docker or memory_mb to 0")
	}
	return nil
}

func buildNativeSandboxCommand(ctx context.Context, cfg config.SandboxConfig, timeout int, interpreter, scriptFile string) (*exec.Cmd, error) {
	interpreterPath, err := exec.LookPath(interpreter)
	if err != nil {
		return nil, fmt.Errorf("sandbox interpreter %q unavailable: %w", interpreter, err)
	}
	workDir, err := filepath.Abs(cfg.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("resolve sandbox workdir: %w", err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(workDir); resolveErr == nil {
		workDir = resolved
	}
	if resolved, resolveErr := filepath.EvalSymlinks(scriptFile); resolveErr == nil {
		scriptFile = resolved
	}
	homeDir, _ := os.UserHomeDir()
	if resolved, resolveErr := filepath.EvalSymlinks(homeDir); resolveErr == nil {
		homeDir = resolved
	}
	if homeDir != "" && strings.HasPrefix(workDir+string(filepath.Separator), homeDir+string(filepath.Separator)) {
		return nil, fmt.Errorf("native macOS sandbox workdir must be outside the user home directory")
	}
	escape := func(value string) string {
		return strings.ReplaceAll(value, `"`, `\"`)
	}
	profile := `(version 1)
(deny default)
(allow process*)
(allow sysctl-read)
(allow file-read*)
(deny file-read* (subpath "` + escape(homeDir) + `"))
(allow file-write* (subpath "` + escape(workDir) + `"))
`
	if cfg.AllowNetwork {
		profile += "(allow network*)\n"
	}
	cpuSeconds := timeout
	if cpuSeconds <= 0 {
		cpuSeconds = 30
	}
	limitAndExec := `ulimit -t "$1" || exit 70; exec "$2" "$3"`
	return exec.CommandContext(ctx, "sandbox-exec", "-p", profile, "/bin/sh", "-c", limitAndExec, "pharus-sandbox",
		strconv.Itoa(cpuSeconds), interpreterPath, scriptFile), nil
}
