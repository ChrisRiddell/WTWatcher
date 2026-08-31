package modules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewLogger_CreatesLogFiles(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer logger.Close()

	for _, name := range []string{"info.log", "warning.log", "error.log"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected log file %s to exist: %v", name, err)
		}
	}
}

func TestLogger_WritesCorrectFiles(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	logger.Info("hello info")
	logger.Warn("hello warn")
	logger.Error("hello error")
	logger.Close()

	for _, tc := range []struct {
		file    string
		keyword string
	}{
		{"info.log", "hello info"},
		{"warning.log", "hello warn"},
		{"error.log", "hello error"},
	} {
		data, err := os.ReadFile(filepath.Join(dir, tc.file))
		if err != nil {
			t.Fatalf("read %s: %v", tc.file, err)
		}
		if !strings.Contains(string(data), tc.keyword) {
			t.Errorf("%s: expected %q, got:\n%s", tc.file, tc.keyword, data)
		}
	}
}

func TestLogger_ThreadSafe(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer logger.Close()

	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func(n int) {
			logger.Info("concurrent", "n", n)
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 50; i++ {
		<-done
	}
}

func TestLogger_Rotate(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer logger.Close()

	logger.Info("info before rotation")
	logger.Warn("warn before rotation")
	logger.Error("error before rotation")

	if err := logger.Rotate(); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	dateKey := time.Now().UTC().Format("2006-01-02")

	// Check that rotated files exist and contain pre-rotation logs.
	for _, tc := range []struct {
		rotatedFile string
		keyword     string
	}{
		{"info-" + dateKey + ".log", "info before rotation"},
		{"warning-" + dateKey + ".log", "warn before rotation"},
		{"error-" + dateKey + ".log", "error before rotation"},
	} {
		path := filepath.Join(dir, tc.rotatedFile)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read rotated file %s: %v", tc.rotatedFile, err)
		}
		if !strings.Contains(string(data), tc.keyword) {
			t.Errorf("%s: expected %q, got %s", tc.rotatedFile, tc.keyword, string(data))
		}
	}

	// Write new logs and verify they go to fresh active log files without old content.
	logger.Info("info after rotation")
	activeInfo, err := os.ReadFile(filepath.Join(dir, "info.log"))
	if err != nil {
		t.Fatalf("read active info.log: %v", err)
	}
	if strings.Contains(string(activeInfo), "info before rotation") {
		t.Errorf("active info.log should not contain pre-rotation content")
	}
	if !strings.Contains(string(activeInfo), "info after rotation") {
		t.Errorf("active info.log missing post-rotation content")
	}
}

func TestLogger_Rotate_EmptyFilesNotArchived(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer logger.Close()

	// Only write to info; warning and error remain empty (0 bytes).
	logger.Info("only info message")

	if err := logger.Rotate(); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	dateKey := time.Now().UTC().Format("2006-01-02")
	if _, err := os.Stat(filepath.Join(dir, "info-"+dateKey+".log")); err != nil {
		t.Errorf("expected info-%s.log to exist: %v", dateKey, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "warning-"+dateKey+".log")); !os.IsNotExist(err) {
		t.Errorf("expected warning-%s.log to not exist for empty log", dateKey)
	}
	if _, err := os.Stat(filepath.Join(dir, "error-"+dateKey+".log")); !os.IsNotExist(err) {
		t.Errorf("expected error-%s.log to not exist for empty log", dateKey)
	}
}

func TestLogger_Rotate_MultipleRotationsSameDay(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer logger.Close()

	dateKey := time.Now().UTC().Format("2006-01-02")

	// First rotation
	logger.Info("first info")
	if err := logger.Rotate(); err != nil {
		t.Fatalf("first Rotate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "info-"+dateKey+".log")); err != nil {
		t.Fatalf("expected info-%s.log to exist: %v", dateKey, err)
	}

	// Second rotation on same date
	logger.Info("second info")
	if err := logger.Rotate(); err != nil {
		t.Fatalf("second Rotate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "info-"+dateKey+"-1.log")); err != nil {
		t.Fatalf("expected disambiguated info-%s-1.log to exist: %v", dateKey, err)
	}
}

func TestLogger_Rotate_ThreadSafe(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer logger.Close()

	done := make(chan struct{})
	for i := 0; i < 30; i++ {
		go func(n int) {
			logger.Info("concurrent write", "n", n)
			done <- struct{}{}
		}(i)
	}

	go func() {
		_ = logger.Rotate()
		done <- struct{}{}
	}()

	for i := 0; i < 31; i++ {
		<-done
	}
}
