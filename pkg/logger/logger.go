package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	globalLogger *slog.Logger
	once         sync.Once
)

// InitLogger initializes the global slog logger.
// It logs to stdout and optionally to a log file in ~/.pharus/logs/pharus.log.
func InitLogger(levelStr string, logFilePath string) *slog.Logger {
	once.Do(func() {
		var level slog.Level
		switch strings.ToLower(levelStr) {
		case "debug":
			level = slog.LevelDebug
		case "warn", "warning":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		default:
			level = slog.LevelInfo
		}

		var writers []io.Writer
		writers = append(writers, os.Stdout)

		if logFilePath != "" {
			dir := filepath.Dir(logFilePath)
			if err := os.MkdirAll(dir, 0755); err == nil {
				f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
				if err == nil {
					writers = append(writers, f)
				}
			}
		}

		multiWriter := io.MultiWriter(writers...)
		handler := slog.NewJSONHandler(multiWriter, &slog.HandlerOptions{
			Level: level,
		})

		globalLogger = slog.New(handler)
		slog.SetDefault(globalLogger)
	})

	if globalLogger == nil {
		globalLogger = slog.Default()
	}
	return globalLogger
}

// Get returns the global logger instance or standard logger if uninitialized.
func Get() *slog.Logger {
	if globalLogger == nil {
		return slog.Default()
	}
	return globalLogger
}

// LogError formats and logs an error cleanly.
func LogError(msg string, err error, args ...any) {
	allArgs := append([]any{"error", err}, args...)
	Get().Error(msg, allArgs...)
}
