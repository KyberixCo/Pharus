package mcp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/KyberixCo/Pharus/internal/config"
)

type CodeExecutionInput struct {
	Language string `json:"language" jsonschema:"Lenguaje del script a ejecutar (python3 o bash)"`
	Script   string `json:"script" jsonschema:"Código o script a ejecutar dentro del entorno sandbox"`
	Timeout  int    `json:"timeout,omitempty" jsonschema:"Tiempo límite de ejecución en segundos"`
}

type CodeExecutionOutput struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Elapsed  string `json:"elapsed"`
	Engine   string `json:"engine"`
}

// SandboxBackend define la interfaz para la ejecución de código aislado.
type SandboxBackend interface {
	Execute(ctx context.Context, input CodeExecutionInput) (CodeExecutionOutput, error)
	Name() string
}

type SandboxRunner struct {
	cfg     config.SandboxConfig
	backend SandboxBackend
}

// NewSandboxRunner crea un SandboxRunner utilizando el directorio especificado o la configuración por defecto.
func NewSandboxRunner(workDir string) (*SandboxRunner, error) {
	cfg := config.SandboxConfig{
		Engine:       "native",
		MemoryMB:     0,
		CPUScores:    1.0,
		AllowNetwork: false,
		WorkDir:      workDir,
	}
	return NewSandboxRunnerWithConfig(cfg)
}

// NewSandboxRunnerWithConfig inicializa el SandboxRunner con la configuración SandboxConfig de Pharus.
func NewSandboxRunnerWithConfig(cfg config.SandboxConfig) (*SandboxRunner, error) {
	if cfg.WorkDir == "" {
		cfg.WorkDir = filepath.Join(os.TempDir(), "pharus_sandbox")
	}
	if err := os.MkdirAll(cfg.WorkDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create sandbox workdir %s: %w", cfg.WorkDir, err)
	}

	var backend SandboxBackend
	switch strings.ToLower(strings.TrimSpace(cfg.Engine)) {
	case "docker":
		if _, err := exec.LookPath("docker"); err != nil {
			return nil, fmt.Errorf("docker sandbox requested but docker is unavailable: %w", err)
		}
		backend = NewDockerBackend(cfg)
	case "native", "":
		if err := nativeSandboxAvailable(cfg); err != nil {
			return nil, err
		}
		backend = NewOSRestrictedBackend(cfg)
	case "auto":
		if dockerReady() {
			backend = NewDockerBackend(cfg)
		} else if err := nativeSandboxAvailable(cfg); err == nil {
			backend = NewOSRestrictedBackend(cfg)
		} else {
			return nil, fmt.Errorf("no strongly isolated sandbox backend is available")
		}
	default:
		return nil, fmt.Errorf("unsupported sandbox engine %q", cfg.Engine)
	}

	return &SandboxRunner{
		cfg:     cfg,
		backend: backend,
	}, nil
}

func (s *SandboxRunner) ExecuteScript(ctx context.Context, input CodeExecutionInput) (CodeExecutionOutput, error) {
	if s.backend == nil {
		return CodeExecutionOutput{}, fmt.Errorf("sandbox backend is not initialized")
	}
	return s.backend.Execute(ctx, input)
}

// --- Driver 1: OSRestrictedBackend (Consumo Mínimo de CPU / RAM) ---

type OSRestrictedBackend struct {
	cfg config.SandboxConfig
}

func NewOSRestrictedBackend(cfg config.SandboxConfig) *OSRestrictedBackend {
	return &OSRestrictedBackend{cfg: cfg}
}

func (b *OSRestrictedBackend) Name() string {
	return "native-restricted"
}

func (b *OSRestrictedBackend) Execute(ctx context.Context, input CodeExecutionInput) (CodeExecutionOutput, error) {
	start := time.Now()

	timeout := input.Timeout
	if timeout <= 0 {
		timeout = 30
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	lang := input.Language
	if lang == "" {
		lang = "python3"
	}

	workDir := b.cfg.WorkDir
	var interpreter string
	var extension string

	switch lang {
	case "python", "python3":
		interpreter = "python3"
		extension = ".py"
	case "bash", "sh":
		interpreter = "bash"
		extension = ".sh"
	default:
		return CodeExecutionOutput{}, fmt.Errorf("unsupported sandbox language: %s", lang)
	}

	scriptFile := filepath.Join(workDir, fmt.Sprintf("script_%d%s", time.Now().UnixNano(), extension))
	if err := os.WriteFile(scriptFile, []byte(input.Script), 0600); err != nil {
		return CodeExecutionOutput{}, fmt.Errorf("failed to write script file: %w", err)
	}
	defer os.Remove(scriptFile)

	cmd, err := buildNativeSandboxCommand(execCtx, b.cfg, timeout, interpreter, scriptFile)
	if err != nil {
		return CodeExecutionOutput{}, err
	}
	cmd.Dir = workDir
	cmd.Env = SanitizeEnv(os.Environ(), workDir)

	stdoutBuf := newLimitedBuffer(1 << 20)
	stderrBuf := newLimitedBuffer(1 << 20)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf
	err = cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return CodeExecutionOutput{
				Stdout:   stdoutBuf.String(),
				Stderr:   err.Error(),
				ExitCode: -1,
				Elapsed:  time.Since(start).String(),
				Engine:   b.Name(),
			}, nil
		}
	}

	return CodeExecutionOutput{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: exitCode,
		Elapsed:  time.Since(start).String(),
		Engine:   b.Name(),
	}, nil
}

// --- Driver 2: DockerBackend (Aislamiento por Contenedor Opcional) ---

type DockerBackend struct {
	cfg config.SandboxConfig
}

func NewDockerBackend(cfg config.SandboxConfig) *DockerBackend {
	return &DockerBackend{cfg: cfg}
}

func (b *DockerBackend) Name() string {
	return "docker"
}

func (b *DockerBackend) Execute(ctx context.Context, input CodeExecutionInput) (CodeExecutionOutput, error) {
	start := time.Now()

	timeout := input.Timeout
	if timeout <= 0 {
		timeout = 30
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	lang := input.Language
	if lang == "" {
		lang = "python3"
	}

	workDir := b.cfg.WorkDir
	scriptFileName := fmt.Sprintf("script_%d.py", time.Now().UnixNano())
	if lang == "bash" || lang == "sh" {
		scriptFileName = fmt.Sprintf("script_%d.sh", time.Now().UnixNano())
	}
	scriptFile := filepath.Join(workDir, scriptFileName)

	if err := os.WriteFile(scriptFile, []byte(input.Script), 0600); err != nil {
		return CodeExecutionOutput{}, fmt.Errorf("failed to write script file: %w", err)
	}
	defer os.Remove(scriptFile)

	args := []string{
		"run", "--rm",
		"--read-only",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"--pids-limit=64",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"-v", fmt.Sprintf("%s:/app:rw", workDir),
		"-w", "/app",
	}

	if !b.cfg.AllowNetwork {
		args = append(args, "--network=none")
	}

	if b.cfg.MemoryMB > 0 {
		args = append(args, fmt.Sprintf("--memory=%dm", b.cfg.MemoryMB))
	}
	if b.cfg.CPUScores > 0 {
		args = append(args, fmt.Sprintf("--cpus=%.2f", b.cfg.CPUScores))
	}

	if lang == "bash" || lang == "sh" {
		args = append(args, "alpine", "sh", scriptFileName)
	} else {
		args = append(args, "python:3.11-slim", "python3", scriptFileName)
	}

	cmd := exec.CommandContext(execCtx, "docker", args...)
	cmd.Dir = workDir

	stdoutBuf := newLimitedBuffer(1 << 20)
	stderrBuf := newLimitedBuffer(1 << 20)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf
	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return CodeExecutionOutput{
				Stdout: stdoutBuf.String(), Stderr: err.Error(), ExitCode: -1,
				Elapsed: time.Since(start).String(), Engine: b.Name(),
			}, fmt.Errorf("docker sandbox failed closed: %w", err)
		}
	}

	return CodeExecutionOutput{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: exitCode,
		Elapsed:  time.Since(start).String(),
		Engine:   b.Name(),
	}, nil
}

func dockerReady() bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "docker", "info").Run() == nil
}

type limitedBuffer struct {
	buf       bytes.Buffer
	remaining int64
	truncated bool
}

func newLimitedBuffer(max int64) *limitedBuffer {
	return &limitedBuffer{remaining: max}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	original := len(p)
	if b.remaining > 0 {
		write := int64(len(p))
		if write > b.remaining {
			write = b.remaining
			b.truncated = true
		}
		_, _ = b.buf.Write(p[:write])
		b.remaining -= write
	} else {
		b.truncated = true
	}
	return original, nil
}

func (b *limitedBuffer) String() string {
	if b.truncated {
		return b.buf.String() + "\n[output truncated]\n"
	}
	return b.buf.String()
}

var _ io.Writer = (*limitedBuffer)(nil)

// --- Sanitización de Entorno ---

// SanitizeEnv filtra activamente cualquier variable de entorno con secretos o claves sensibles.
func SanitizeEnv(rawEnv []string, workDir string) []string {
	var clean []string
	sensitiveKeywords := []string{
		"PHARUS_", "MINIMAX_", "AWS_", "SECRET", "API_KEY", "TOKEN", "PASSWORD", "PASSWD", "AUTH", "CREDENTIAL", "PRIVATE",
	}

	for _, item := range rawEnv {
		pair := strings.SplitN(item, "=", 2)
		if len(pair) < 2 {
			continue
		}
		key := strings.ToUpper(pair[0])

		isSensitive := false
		for _, kw := range sensitiveKeywords {
			if strings.Contains(key, kw) {
				isSensitive = true
				break
			}
		}

		if !isSensitive && key != "HOME" && key != "TMPDIR" {
			clean = append(clean, item)
		}
	}

	clean = append(clean, fmt.Sprintf("HOME=%s", workDir))
	clean = append(clean, fmt.Sprintf("TMPDIR=%s", workDir))
	clean = append(clean, "PYTHONUNBUFFERED=1")

	return clean
}
