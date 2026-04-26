package modules

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestServer_ServesFiles(t *testing.T) {
	dir := t.TempDir()

	// Create a test file in the public dir.
	if err := os.WriteFile(dir+"/hello.txt", []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Verify our handler serves it.
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(dir)))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/hello.txt")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}
}

func TestServer_Returns404ForMissingFile(t *testing.T) {
	dir := t.TempDir()
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(dir)))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/missing.txt")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status: want 404, got %d", resp.StatusCode)
	}
}

func TestNewServer_Constructor(t *testing.T) {
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
}
