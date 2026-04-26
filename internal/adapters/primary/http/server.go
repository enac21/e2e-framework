package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/services"
)

const maxStoredResults = 100

type Server struct {
	httpServer   *http.Server
	orchestrator *services.Orchestrator
	tests        map[string]domain.TestDefinition
	results      map[string]*domain.TestResult
	resultOrder  []string
	mu           sync.RWMutex
}

func NewServer(port int, orchestrator *services.Orchestrator, tests map[string]domain.TestDefinition) *Server {
	s := &Server{
		orchestrator: orchestrator,
		tests:        tests,
		results:      make(map[string]*domain.TestResult),
		resultOrder:  make([]string, 0),
	}

	mux := http.NewServeMux()

	log.Printf("[HTTP API] Registered endpoint: GET /health")
	mux.HandleFunc("/health", s.handleHealth)

	log.Printf("[HTTP API] Registered endpoint: POST /run")
	mux.HandleFunc("/run", s.handleRun)

	log.Printf("[HTTP API] Registered endpoint: GET /results")
	log.Printf("[HTTP API] Registered endpoint: GET /results/{run_id}")
	mux.HandleFunc("/results", s.handleResults)
	mux.HandleFunc("/results/", s.handleResultByID)

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

func (s *Server) storeResult(res *domain.TestResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.results[res.RunID]; !exists {
		s.resultOrder = append(s.resultOrder, res.RunID)
	}
	s.results[res.RunID] = res

	if len(s.resultOrder) > maxStoredResults {
		oldest := s.resultOrder[0]
		s.resultOrder = s.resultOrder[1:]
		delete(s.results, oldest)
	}
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	ctx := context.WithoutCancel(r.Context())

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)

		return
	}

	id := r.URL.Query().Get("id")
	def, ok := s.tests[id]
	if !ok {
		http.Error(w, fmt.Sprintf("test %q not found", id), http.StatusNotFound)

		return
	}

	runID, resultCh := s.orchestrator.RunTest(ctx, def)

	if def.Async {
		s.storeResult(&domain.TestResult{
			TestID:    def.ID,
			RunID:     runID,
			Status:    domain.StatusRunning,
			StartedAt: time.Now(),
		})

		go func() {
			s.storeResult(<-resultCh)
		}()

		respondJSON(w, http.StatusAccepted, map[string]string{
			"run_id": runID,
			"status": domain.StatusRunning,
		})

		return
	}

	result := <-resultCh
	s.storeResult(result)
	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleResultByID(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimPrefix(r.URL.Path, "/results/")
	if runID == "" {
		http.Error(w, "run_id required", http.StatusBadRequest)

		return
	}

	s.mu.RLock()
	result, ok := s.results[runID]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, fmt.Sprintf("run %q not found", runID), http.StatusNotFound)

		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleResults(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	list := make([]*domain.TestResult, 0, len(s.resultOrder))
	for _, id := range s.resultOrder {
		list = append(list, s.results[id])
	}
	s.mu.RUnlock()
	respondJSON(w, http.StatusOK, list)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
