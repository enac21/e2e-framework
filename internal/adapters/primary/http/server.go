package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/services"
)

type Server struct {
	httpServer   *http.Server
	orchestrator *services.Orchestrator
	tests        map[string]domain.TestDefinition
	results      []*domain.TestResult
	mu           sync.RWMutex
}

func NewServer(port int, orchestrator *services.Orchestrator, tests map[string]domain.TestDefinition) *Server {
	s := &Server{
		orchestrator: orchestrator,
		tests:        tests,
		results:      make([]*domain.TestResult, 0),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/run", s.handleRun)
	mux.HandleFunc("/results", s.handleResults)

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	return s
}

func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) addResult(res *domain.TestResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results = append(s.results, res)
	// keep only the last 100 results to avoid memory leak
	if len(s.results) > 100 {
		s.results = s.results[1:]
	}
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
