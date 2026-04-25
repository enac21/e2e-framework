package webhook

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"e2e-framework/internal/core/ports"
)

type Server struct {
	httpServer *http.Server
	store      ports.Store
	extractors map[string]Extractor
}

func NewServer(port int, store ports.Store) *Server {
	mux := http.NewServeMux()

	s := &Server{
		store:      store,
		extractors: make(map[string]Extractor),
	}

	mux.HandleFunc("/webhook/", s.handleWebhook)

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	return s
}

func (s *Server) RegisterExtractor(path string, ext Extractor) {
	s.extractors[path] = ext
}

func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	// path looks like /webhook/{provider}
	provider := r.URL.Path[len("/webhook/"):]

	extractor, exists := s.extractors[provider]
	if !exists {
		http.Error(w, "unknown provider", http.StatusNotFound)
		return
	}

	msg, err := extractor.Extract(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if msg.RunID == "" || msg.RunID == "unknown" {
		// Log error but return 200 OK to the provider so they don't retry
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := s.store.Deposit(r.Context(), msg); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
