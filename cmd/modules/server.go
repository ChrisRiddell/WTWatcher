package modules

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Server is a minimal HTTP file server.
type Server struct {
	mu     sync.Mutex
	port   int
	dir    string
	logger *Logger
	srv    *http.Server
}

// NewServer creates a Server that serves files from dir on the given port.
func NewServer(port int, dir string, logger *Logger) *Server {
	return &Server{port: port, dir: dir, logger: logger}
}

// Handler constructs and returns the HTTP handler for the server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	fileServer := http.FileServer(http.Dir(s.dir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics.json" {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		}
		fileServer.ServeHTTP(w, r)
	})
	return mux
}

// Start begins listening on loopback (127.0.0.1). It blocks until the server exits.
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

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	srv := s.srv
	s.mu.Unlock()

	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}
