package webhook

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"e2e-framework/internal/core/ports"
)

type Server struct {
	httpServer *http.Server
	store      ports.Store
	extractors map[string]ports.Extractor
}

func NewServer(port int, store ports.Store) *Server {
	mux := http.NewServeMux()

	s := &Server{
		store:      store,
		extractors: make(map[string]ports.Extractor),
	}

	mux.HandleFunc("/webhook/", s.handleWebhook)

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	return s
}

func (s *Server) RegisterExtractor(path string, ext ports.Extractor) {
	log.Printf("[Webhook API] Registered endpoint: POST /webhook/%s", path)
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

// handleWebhook godoc
// @Summary Receive webhook from provider
// @Description Deposit messages from providers into the store
// @Tags Webhooks
// @Param provider path string true "Provider name (e.g., twilio, meta)"
// @Produce json
// @Success 202
// @Failure 404 {string} string "Unknown provider"
// @Failure 400 {string} string "Error extracting message data"
// @Failure 500 {string} string "Internal server error"
// @Router /webhook/{provider} [post]
func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := context.WithoutCancel(r.Context())

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
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	if err := s.store.Deposit(ctx, msg); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}
