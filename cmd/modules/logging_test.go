package modules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
