//go:build !darwin && !linux

package mcp

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/KyberixCo/Pharus/internal/config"
)

func nativeSandboxAvailable(config.SandboxConfig) error {
	return fmt.Errorf("native sandbox is unsupported on this operating system; use docker")
}

func buildNativeSandboxCommand(context.Context, config.SandboxConfig, int, string, string) (*exec.Cmd, error) {
	return nil, nativeSandboxAvailable(config.SandboxConfig{})
}
