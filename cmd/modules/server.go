package modules

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Server encapsulates an HTTP static file server configured with security headers,
// directory listing prevention, and live metrics cache suppression.
type Server struct {
	mu     sync.Mutex
	port   int
	dir    string
	logger *Logger
	srv    *http.Server
}

// NewServer creates an HTTP Server instance configured to serve static assets from dir.
func NewServer(port int, dir string, logger *Logger) *Server {
	return &Server{port: port, dir: dir, logger: logger}
}

// Handler constructs the HTTP routing handler with security policies and header configurations.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	fileServer := http.FileServer(http.Dir(s.dir))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// ── Security headers ───────────────────────────────────────────────
		// Prevent MIME-type sniffing by browsers.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Prevent clickjacking by restricting framing to same origin.
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		// Content-Security-Policy (CSP) tailored to client runtime requirements:
		//   - Google Fonts (stylesheets and font files)
		//   - jsdelivr CDN (chart.js and luxon modules loaded via importmap)
		//   - 'unsafe-inline' for inline styles and the inline importmap <script> block
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; "+
				"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; "+
				"font-src 'self' https://fonts.gstatic.com; "+
				"connect-src 'self' https://cdn.jsdelivr.net",
		)

		// ── Prevent directory listings ─────────────────────────────────────
		// Standard Go http.FileServer renders an HTML directory index if the URL resolves
		// to a directory without index.html. To prevent disclosing filesystem contents,
		// intercept subdirectory requests and reject directory queries with 403 Forbidden.
		if r.URL.Path != "/" {
			absPath := filepath.Join(s.dir, filepath.Clean("/"+r.URL.Path))
			info, err := os.Stat(absPath)
			if err == nil && info.IsDir() {
				http.Error(w, "403 Forbidden", http.StatusForbidden)
				return
			}
		}

		// ── No-cache for metrics.json ──────────────────────────────────────
		// Ensure clients and proxy caches always fetch the latest metrics.json
		// rather than caching stale network telemetry.
		if r.URL.Path == "/metrics.json" {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		}

		fileServer.ServeHTTP(w, r)
	})
	return mux
}

// Start binds to loopback interface (127.0.0.1) on the configured port and starts serving HTTP requests.
// It blocks until the server encounters an error or is shut down.
func (s *Server) Start() error {
	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	url := fmt.Sprintf("http://%s", addr)
	fmt.Printf("HTTP server listening on %s\n", url)
	s.logger.Info("http server started", "url", url)

	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	s.mu.Lock()
	s.srv = srv
	s.mu.Unlock()

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown gracefully terminates the HTTP server within the provided context deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	srv := s.srv
	s.mu.Unlock()

	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}
