package modules

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// Logger wraps three slog.Logger instances for info, warning, and error levels.
// All writes are serialised through a Mutex.
type Logger struct {
	mu      sync.Mutex
	info    *slog.Logger
	warning *slog.Logger
	errLog  *slog.Logger

	// keep file handles so we can close them later
	infoFile    *os.File
	warningFile *os.File
	errorFile   *os.File
}

// NewLogger creates (or opens) the three log files under logDir and returns a
// ready-to-use Logger. logDir is created if it does not already exist.
func NewLogger(logDir string) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	open := func(name string) (*os.File, error) {
		return os.OpenFile(filepath.Join(logDir, name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	}

	infoFile, err := open("info.log")
	if err != nil {
		return nil, fmt.Errorf("open info.log: %w", err)
	}

	warnFile, err := open("warning.log")
	if err != nil {
		infoFile.Close()
		return nil, fmt.Errorf("open warning.log: %w", err)
	}

	errFile, err := open("error.log")
	if err != nil {
		infoFile.Close()
		warnFile.Close()
		return nil, fmt.Errorf("open error.log: %w", err)
	}

	newHandler := func(w io.Writer, level slog.Level) *slog.Logger {
		return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
	}

	return &Logger{
		info:        newHandler(infoFile, slog.LevelInfo),
		warning:     newHandler(warnFile, slog.LevelWarn),
		errLog:      newHandler(errFile, slog.LevelError),
		infoFile:    infoFile,
		warningFile: warnFile,
		errorFile:   errFile,
	}, nil
}

// Close flushes and closes all log files.
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infoFile.Close()
	l.warningFile.Close()
	l.errorFile.Close()
}

// Info logs an informational message.
func (l *Logger) Info(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.info.Info(msg, args...)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warning.Warn(msg, args...)
}

// Error logs an error message.
func (l *Logger) Error(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errLog.Error(msg, args...)
}
