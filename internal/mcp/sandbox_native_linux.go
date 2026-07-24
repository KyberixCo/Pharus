//go:build linux

package mcp

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/KyberixCo/Pharus/internal/config"
)

func nativeSandboxAvailable(config.SandboxConfig) error {
	if _, err := exec.LookPath("bwrap"); err != nil {
		return fmt.Errorf("native sandbox requires bubblewrap (bwrap): %w", err)
	}
	if _, err := exec.LookPath("prlimit"); err != nil {
		return fmt.Errorf("native sandbox requires prlimit: %w", err)
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
	args := []string{
		"--die-with-parent", "--new-session", "--unshare-all",
		"--ro-bind", "/usr", "/usr",
		"--ro-bind", "/bin", "/bin",
		"--ro-bind-try", "/lib", "/lib",
		"--ro-bind-try", "/lib64", "/lib64",
		"--dev", "/dev", "--proc", "/proc",
		"--tmpfs", "/tmp",
		"--bind", workDir, workDir,
		"--chdir", workDir,
	}
	if cfg.AllowNetwork {
		args = append(args, "--share-net")
	}
	memoryBytes := int64(cfg.MemoryMB) * 1024 * 1024
	if memoryBytes <= 0 {
		memoryBytes = 512 * 1024 * 1024
	}
	if timeout <= 0 {
		timeout = 30
	}
	args = append(args, "prlimit",
		"--as="+strconv.FormatInt(memoryBytes, 10),
		"--cpu="+strconv.Itoa(timeout),
		"--nproc=64",
		"--", interpreterPath, scriptFile)
	return exec.CommandContext(ctx, "bwrap", args...), nil
}
