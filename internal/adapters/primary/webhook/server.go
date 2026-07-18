package webhook

import (
	"context"
	"log"
	"net/http"

	"e2e-framework/internal/core/ports"
)

type Server struct {
	store      ports.Store
	extractors map[string]ports.Extractor
}

func NewServer(store ports.Store) *Server {
	return &Server{
		store:      store,
		extractors: make(map[string]ports.Extractor),
	}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) *Server {
	mux.HandleFunc("/webhook/", s.handleWebhook)

	return s
}

func (s *Server) RegisterExtractor(path string, ext ports.Extractor) *Server {
	log.Printf("[Webhook API] Registered endpoint: POST /webhook/%s", path)
	s.extractors[path] = ext

	return s
}

// handleWebhook godoc
// @Summary Receive webhook from provider
// @Description Deposit messages from providers into the store
// @Tags Webhooks
// @Param provider path string true "Provider name (e.g., twilio, meta)"
// @Produce json
// @Success 202
// @Failure 401 {string} string "Unauthorized"
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

	//TODO - Add auth middleware per provider.
	msg, err := extractor.Extract(r)
	if err != nil {
		//TODO - Error handler in base of the domain error

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

func (s *Server) errorHandler(_ http.ResponseWriter, _ *http.Request, _ error) {
	//WIP
}
