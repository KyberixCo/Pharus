package daemon

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/KyberixCo/Pharus/internal/config"
)

const launchdPlistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.pharus.daemon</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{ .ExecutablePath }}</string>
        <string>daemon</string>
        <string>run</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>{{ .LogPath }}</string>
    <key>StandardErrorPath</key>
    <string>{{ .ErrLogPath }}</string>
    <key>WorkingDirectory</key>
    <string>{{ .BaseDir }}</string>
</dict>
</plist>
`

type LaunchdData struct {
	ExecutablePath string
	LogPath        string
	ErrLogPath     string
	BaseDir        string
}

// GetPlistPath returns the target plist path in user's LaunchAgents.
func GetPlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", "com.pharus.daemon.plist"), nil
}

// InstallLaunchdService installs and loads the launchd plist service.
func InstallLaunchdService(cfg *config.Config) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	plistPath, err := GetPlistPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(plistPath), 0755); err != nil {
		return fmt.Errorf("failed to create LaunchAgents dir: %w", err)
	}

	tmpl, err := template.New("plist").Parse(launchdPlistTemplate)
	if err != nil {
		return err
	}

	data := LaunchdData{
		ExecutablePath: exePath,
		LogPath:        filepath.Join(cfg.LogsDir, "daemon_stdout.log"),
		ErrLogPath:     filepath.Join(cfg.LogsDir, "daemon_stderr.log"),
		BaseDir:        cfg.BaseDir,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}

	if err := os.WriteFile(plistPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write launchd plist: %w", err)
	}

	// Load service using launchctl
	cmd := exec.Command("launchctl", "load", "-w", plistPath)
	_ = cmd.Run()

	return nil
}

// UninstallLaunchdService unloads and removes the launchd service.
func UninstallLaunchdService() error {
	plistPath, err := GetPlistPath()
	if err != nil {
		return err
	}

	_ = exec.Command("launchctl", "unload", "-w", plistPath).Run()
	_ = os.Remove(plistPath)
	return nil
}
