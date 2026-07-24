package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KyberixCo/Pharus/internal/config"
)

func TestSandboxRunner(t *testing.T) {
	tmpDir := t.TempDir()
	runner, err := NewSandboxRunner(tmpDir)
	if err != nil {
		t.Fatalf("failed to create sandbox runner: %v", err)
	}

	ctx := context.Background()
	output, err := runner.ExecuteScript(ctx, CodeExecutionInput{
		Language: "python3",
		Script:   "print('Pharus Code Execution MCP Active')",
	})

	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}

	if output.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d. Stderr: %s", output.ExitCode, output.Stderr)
	}

	if output.Stdout != "Pharus Code Execution MCP Active\n" {
		t.Errorf("unexpected stdout: %q", output.Stdout)
	}

	if output.Engine != "native-restricted" {
		t.Errorf("expected engine 'native-restricted', got %q", output.Engine)
	}
}

func TestDockerBackendFailsClosedWithoutNativeFallback(t *testing.T) {
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	if err := os.WriteFile(dockerPath, []byte("#!/bin/sh\nexit 125\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	runner, err := NewSandboxRunnerWithConfig(config.SandboxConfig{
		Engine: "docker", WorkDir: t.TempDir(), MemoryMB: 64, CPUScores: 0.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := runner.ExecuteScript(context.Background(), CodeExecutionInput{
		Language: "bash", Script: "echo must-not-run",
	})
	if err != nil {
		t.Fatalf("docker process exit should be reported as sandbox output: %v", err)
	}
	if output.Engine != "docker" || output.ExitCode != 125 || strings.Contains(output.Stdout, "must-not-run") {
		t.Fatalf("docker execution unexpectedly fell back: %+v", output)
	}
}

func TestSandboxRunnerBash(t *testing.T) {
	tmpDir := t.TempDir()
	runner, err := NewSandboxRunner(tmpDir)
	if err != nil {
		t.Fatalf("failed to create sandbox runner: %v", err)
	}

	ctx := context.Background()
	output, err := runner.ExecuteScript(ctx, CodeExecutionInput{
		Language: "bash",
		Script:   "echo 'Hello Bash Sandbox'",
	})

	if err != nil {
		t.Fatalf("unexpected bash execution error: %v", err)
	}

	if output.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d. Stderr: %s", output.ExitCode, output.Stderr)
	}

	if strings.TrimSpace(output.Stdout) != "Hello Bash Sandbox" {
		t.Errorf("unexpected stdout: %q", output.Stdout)
	}
}

func TestSanitizeEnv(t *testing.T) {
	rawEnv := []string{
		"PATH=/usr/bin:/bin",
		"PHARUS_MINIMAX_API_KEY=secret_key_12345",
		"AWS_SECRET_ACCESS_KEY=aws_secret",
		"DAEMON_TOKEN=abcde",
		"USER=julieta",
		"HOME=/Users/julieta",
	}
	workDir := "/tmp/pharus_sandbox_test"

	cleanEnv := SanitizeEnv(rawEnv, workDir)

	cleanStr := strings.Join(cleanEnv, "\n")
	if strings.Contains(cleanStr, "PHARUS_MINIMAX_API_KEY") {
		t.Errorf("SanitizeEnv leaked PHARUS_MINIMAX_API_KEY")
	}
	if strings.Contains(cleanStr, "AWS_SECRET_ACCESS_KEY") {
		t.Errorf("SanitizeEnv leaked AWS_SECRET_ACCESS_KEY")
	}
	if strings.Contains(cleanStr, "DAEMON_TOKEN") {
		t.Errorf("SanitizeEnv leaked DAEMON_TOKEN")
	}
	if !strings.Contains(cleanStr, "HOME=/tmp/pharus_sandbox_test") {
		t.Errorf("SanitizeEnv failed to set isolated HOME")
	}
	if !strings.Contains(cleanStr, "TMPDIR=/tmp/pharus_sandbox_test") {
		t.Errorf("SanitizeEnv failed to set isolated TMPDIR")
	}
}

func TestSandboxRunnerTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	runner, err := NewSandboxRunnerWithConfig(config.SandboxConfig{
		Engine:  "native",
		WorkDir: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create sandbox runner: %v", err)
	}

	ctx := context.Background()
	output, err := runner.ExecuteScript(ctx, CodeExecutionInput{
		Language: "python3",
		Script:   "import time; time.sleep(10)",
		Timeout:  1,
	})

	if err != nil {
		t.Fatalf("unexpected error handling timeout: %v", err)
	}

	if output.ExitCode == 0 {
		t.Errorf("expected non-zero exit code due to timeout, got 0")
	}
}
