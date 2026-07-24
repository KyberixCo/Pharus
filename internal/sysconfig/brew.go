package sysconfig

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// BrewManager handles interacting with Homebrew on macOS.
type BrewManager struct{}

// NewBrewManager creates a new BrewManager instance.
func NewBrewManager() *BrewManager {
	return &BrewManager{}
}

// IsBrewInstalled checks if brew command exists in PATH.
func (b *BrewManager) IsBrewInstalled(ctx context.Context) bool {
	_, err := exec.LookPath("brew")
	return err == nil
}

// IsFormulaInstalled checks if a formula or cask is installed.
func (b *BrewManager) IsFormulaInstalled(ctx context.Context, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, "brew", "list", name)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	return false, nil
}

// IsServiceRunning checks if a brew service is running.
func (b *BrewManager) IsServiceRunning(ctx context.Context, serviceName string) (bool, error) {
	cmd := exec.CommandContext(ctx, "brew", "services", "list")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return false, err
	}
	return bytes.Contains(out.Bytes(), []byte(serviceName+" started")), nil
}

// InstallFormula installs a homebrew formula.
func (b *BrewManager) InstallFormula(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "brew", "install", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to brew install %s: %s (%w)", name, stderr.String(), err)
	}
	return nil
}

// StartService starts a homebrew background service.
func (b *BrewManager) StartService(ctx context.Context, serviceName string) error {
	cmd := exec.CommandContext(ctx, "brew", "services", "start", serviceName)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start brew service %s: %s (%w)", serviceName, stderr.String(), err)
	}
	return nil
}
