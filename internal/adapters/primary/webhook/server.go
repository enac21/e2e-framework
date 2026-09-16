package webhook

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type Server struct {
	ingestor   ports.MessageIngestor
	extractors map[string]ports.Extractor
}

func NewServer(ingestor ports.MessageIngestor) (*Server, error) {
	if ingestor == nil {
		return nil, fmt.Errorf("%w: message ingestor is required", domain.ErrConfiguration)
	}
	return &Server{
		ingestor:   ingestor,
		extractors: make(map[string]ports.Extractor),
	}, nil
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/webhook/", s.handleWebhook)
}

func (s *Server) RegisterExtractor(path string, ext ports.Extractor) {
	log.Printf("[Webhook API] Registered endpoint: POST /webhook/%s", path)
	s.extractors[path] = ext
}

// handleWebhook godoc
// @Summary Receive webhook from provider
// @Description Deposit messages from providers into the store. This endpoint feeds the webhook receiver: test steps that declare receiver.type: webhook poll the store until a message deposited here matches the run (msg.RunID equals the test run id), so provider callbacks complete the async step.
// @Tags Webhooks
// @Param provider path string true "Provider name (e.g., twilio, meta, generic (for non-specific providers)"
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

	var (
		msg *domain.Message
		err error
	)

	extractor, exists := s.extractors[provider]
	if !exists {
		http.Error(w, "unknown provider", http.StatusNotFound)

		return
	}

	msg, err = extractor.Extract(r)
	if err != nil {
		if errors.Is(err, domain.ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}

		return
	}

	if err := s.ingestor.Ingest(ctx, msg); err != nil {
		if errors.Is(err, domain.ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) errorHandler(_ http.ResponseWriter, _ *http.Request, _ error) {
	//WIP
}
