package modules

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// logFileNames defines the dedicated log filenames managed by the Logger.
var logFileNames = []string{"info.log", "warning.log", "error.log"}

// Logger wraps three distinct slog.Logger instances routing structured JSON output to
// dedicated log files for info, warning, and error levels.
// All write and rotation operations are serialized through a mutex to prevent race conditions or interleaved log writes.
type Logger struct {
	mu      sync.Mutex
	logDir  string
	info    *slog.Logger
	warning *slog.Logger
	errLog  *slog.Logger

	// Keep underlying OS file handles to flush and close descriptors cleanly on shutdown or rotation.
	files []*os.File
}

// NewLogger creates the log directory if missing and opens info.log, warning.log,
// and error.log in append mode. It configures structured JSON handlers for each file stream.
func NewLogger(logDir string) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	l := &Logger{
		logDir: logDir,
	}

	if err := l.openFiles(); err != nil {
		return nil, err
	}

	return l, nil
}

// openFiles opens the three level-specific log files and configures their JSON slog handlers.
// Caller must hold l.mu if called after Logger initialization.
func (l *Logger) openFiles() error {
	var files []*os.File
	cleanup := func() {
		for _, f := range files {
			_ = f.Close()
		}
	}

	open := func(name string) (*os.File, error) {
		f, err := os.OpenFile(filepath.Join(l.logDir, name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", name, err)
		}
		files = append(files, f)
		return f, nil
	}

	infoFile, err := open("info.log")
	if err != nil {
		return err
	}

	warnFile, err := open("warning.log")
	if err != nil {
		cleanup()
		return err
	}

	errFile, err := open("error.log")
	if err != nil {
		cleanup()
		return err
	}

	newHandler := func(w io.Writer, level slog.Level) *slog.Logger {
		return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
	}

	l.info = newHandler(infoFile, slog.LevelInfo)
	l.warning = newHandler(warnFile, slog.LevelWarn)
	l.errLog = newHandler(errFile, slog.LevelError)
	l.files = files
	return nil
}

// Rotate safely flushes and closes active log files, renames non-empty logs with a date suffix
// (e.g. info-2006-01-02.log), and opens fresh log files for active writing.
func (l *Logger) Rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Flush and close all open file handles before renaming.
	var closeErrs []error
	for _, f := range l.files {
		_ = f.Sync()
		if err := f.Close(); err != nil {
			closeErrs = append(closeErrs, err)
		}
	}
	l.files = nil

	// Date format for rotated files: e.g. "info-2006-01-02.log"
	dateKey := time.Now().UTC().Format("2006-01-02")

	var renameErrs []error
	for _, name := range logFileNames {
		src := filepath.Join(l.logDir, name)
		fi, err := os.Stat(src)
		if err != nil {
			// File does not exist, nothing to rotate.
			continue
		}
		// If the file is empty, skip renaming so we don't produce redundant empty archives.
		if fi.Size() == 0 {
			continue
		}

		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		dst := filepath.Join(l.logDir, fmt.Sprintf("%s-%s%s", base, dateKey, ext))

		// If a file with the target name already exists (e.g. rotated again on the same day),
		// disambiguate with an incremental numeric suffix.
		if _, err := os.Stat(dst); err == nil {
			for i := 1; ; i++ {
				candidate := filepath.Join(l.logDir, fmt.Sprintf("%s-%s-%d%s", base, dateKey, i, ext))
				if _, err := os.Stat(candidate); os.IsNotExist(err) {
					dst = candidate
					break
				}
			}
		}

		if err := os.Rename(src, dst); err != nil {
			renameErrs = append(renameErrs, fmt.Errorf("rename %s to %s: %w", src, dst, err))
		}
	}

	// Open fresh active log files and reinitialize slog handlers.
	openErr := l.openFiles()

	var allErrs []error
	allErrs = append(allErrs, closeErrs...)
	allErrs = append(allErrs, renameErrs...)
	if openErr != nil {
		allErrs = append(allErrs, openErr)
	}

	return errors.Join(allErrs...)
}

// Close flushes and closes all underlying log file descriptors, combining any closing errors.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	var errs []error
	for _, f := range l.files {
		_ = f.Sync()
		if err := f.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	l.files = nil
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
