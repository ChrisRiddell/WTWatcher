package modules

import (
	"fmt"
	"net/http"
)

// Server is a minimal HTTP file server.
type Server struct {
	port   int
	dir    string
	logger *Logger
}

// NewServer creates a Server that serves files from dir on the given port.
func NewServer(port int, dir string, logger *Logger) *Server {
	return &Server{port: port, dir: dir, logger: logger}
}

// Start begins listening. It blocks until the server exits.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	url := fmt.Sprintf("http://127.0.0.1%s", addr)
	fmt.Printf("HTTP server listening on %s\n", url)
	s.logger.Info("http server started", "url", url)

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

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	return srv.ListenAndServe()
}
