package modules

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// Logger wraps three distinct slog.Logger instances routing structured JSON output to
// dedicated log files for info, warning, and error levels.
// All write operations are serialized through a mutex to prevent race conditions or interleaved log writes.
type Logger struct {
	mu      sync.Mutex
	info    *slog.Logger
	warning *slog.Logger
	errLog  *slog.Logger

	// Keep underlying OS file handles to flush and close descriptors cleanly on shutdown.
	files []*os.File
}

// NewLogger creates the log directory if missing and opens info.log, warning.log,
// and error.log in append mode. It configures structured JSON handlers for each file stream.
func NewLogger(logDir string) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	var files []*os.File
	cleanup := func() {
		for _, f := range files {
			f.Close()
		}
	}

	open := func(name string) (*os.File, error) {
		f, err := os.OpenFile(filepath.Join(logDir, name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", name, err)
		}
		files = append(files, f)
		return f, nil
	}

	infoFile, err := open("info.log")
	if err != nil {
		return nil, err
	}

	warnFile, err := open("warning.log")
	if err != nil {
		cleanup()
		return nil, err
	}

	errFile, err := open("error.log")
	if err != nil {
		cleanup()
		return nil, err
	}

	newHandler := func(w io.Writer, level slog.Level) *slog.Logger {
		return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
	}

	return &Logger{
		info:    newHandler(infoFile, slog.LevelInfo),
		warning: newHandler(warnFile, slog.LevelWarn),
		errLog:  newHandler(errFile, slog.LevelError),
		files:   files,
	}, nil
}

// Close flushes and closes all underlying log file descriptors, combining any closing errors.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	var errs []error
	for _, f := range l.files {
		if err := f.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Info logs a structured informational message to info.log.
func (l *Logger) Info(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.info.Info(msg, args...)
}

// Warn logs a structured warning message to warning.log.
func (l *Logger) Warn(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warning.Warn(msg, args...)
}

// Error logs a structured error message to error.log.
func (l *Logger) Error(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errLog.Error(msg, args...)
}
