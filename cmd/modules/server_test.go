package modules

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestServer_ServesFiles(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(t.TempDir())
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer logger.Close()

	if err := os.WriteFile(dir+"/hello.txt", []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(8080, dir, logger)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/hello.txt", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}
	if w.Body.String() != "world" {
		t.Errorf("body: want %q, got %q", "world", w.Body.String())
	}
}

func TestServer_MetricsNoCacheHeaders(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(t.TempDir())
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer logger.Close()

	if err := os.WriteFile(dir+"/metrics.json", []byte(`{"test": true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(8080, dir, logger)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/metrics.json", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache, no-store, must-revalidate" {
		t.Errorf("Cache-Control: want %q, got %q", "no-cache, no-store, must-revalidate", got)
	}
	if got := resp.Header.Get("Pragma"); got != "no-cache" {
		t.Errorf("Pragma: want %q, got %q", "no-cache", got)
	}
	if got := resp.Header.Get("Expires"); got != "0" {
		t.Errorf("Expires: want %q, got %q", "0", got)
	}
}

func TestServer_Returns404ForMissingFile(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(t.TempDir())
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer logger.Close()

	srv := NewServer(8080, dir, logger)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/missing.txt", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status: want 404, got %d", resp.StatusCode)
	}
}

func TestNewServer_ConstructorAndShutdown(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(t.TempDir())
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer logger.Close()

	srv := NewServer(8080, dir, logger)
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
	if srv.port != 8080 {
		t.Errorf("port: want 8080, got %d", srv.port)
	}
	if srv.dir != dir {
		t.Errorf("dir: want %q, got %q", dir, srv.dir)
	}

	// Shutdown before start should succeed without error
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown unstarted: want nil, got %v", err)
	}
}

func TestServer_SecurityHeaders(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(t.TempDir())
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer logger.Close()

	if err := os.WriteFile(dir+"/index.html", []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(8080, dir, logger)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	checks := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "SAMEORIGIN",
		"Content-Security-Policy": "default-src 'self'; " +
			"script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; " +
			"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
			"font-src 'self' https://fonts.gstatic.com; " +
			"connect-src 'self' https://cdn.jsdelivr.net",
	}
	for header, want := range checks {
		if got := resp.Header.Get(header); got != want {
			t.Errorf("%s: want %q, got %q", header, want, got)
		}
	}
}

func TestServer_DirectoryListingForbidden(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(t.TempDir())
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer logger.Close()

	// Create a subdirectory — requesting it should return 403, not a listing.
	if err := os.Mkdir(dir+"/subdir", 0o755); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(8080, dir, logger)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/subdir", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("directory listing: want 403, got %d", resp.StatusCode)
	}
}
