package api

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

	_ "e2e-framework/docs"

	httpSwagger "github.com/swaggo/http-swagger"
)

const (
	maxStoredResults = 100

	unauthorizedMessage = "unauthorized"
)

type Server struct {
	cfg          *Config
	mux          *http.ServeMux
	httpServer   *http.Server
	orchestrator *services.Orchestrator
	tests        map[string]domain.TestDefinition
	results      map[string]*domain.TestResult
	resultOrder  []string
	mu           sync.RWMutex
}

type Config struct {
	Port       int
	AuthEnable bool
	JWTSecret  string
}

func (s *Server) Mux() *http.ServeMux {
	return s.mux
}

func NewServer(cfg *Config, orchestrator *services.Orchestrator, tests map[string]domain.TestDefinition) *Server {
	mux := http.NewServeMux()

	s := &Server{
		cfg:          cfg,
		mux:          mux,
		orchestrator: orchestrator,
		tests:        tests,
		results:      make(map[string]*domain.TestResult),
		resultOrder:  make([]string, 0),
	}

	log.Printf("[HTTP API] Registered endpoint: GET /health")
	mux.HandleFunc("/health", s.handleHealth)

	log.Printf("[HTTP API] Registered endpoint: POST /run")
	mux.HandleFunc("/run", s.authMiddleware(s.handleRun))

	log.Printf("[HTTP API] Registered endpoint: POST /run-sequence")
	mux.HandleFunc("/run-sequence", s.authMiddleware(s.handleRunSequence))

	log.Printf("[HTTP API] Registered endpoint: GET /results")
	mux.HandleFunc("/results", s.authMiddleware(s.handleResults))

	log.Printf("[HTTP API] Registered endpoint: GET /results/{run_id}")
	mux.HandleFunc("/results/", s.authMiddleware(s.handleResultByID))

	log.Printf("[HTTP API] Registered endpoint: GET /swagger/*")
	mux.Handle("/swagger/", s.authMiddleware(httpSwagger.WrapHandler.ServeHTTP))

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: mux,
	}

	return s
}

func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.AuthEnable {
			next(w, r)

			return
		}

		token := strings.TrimPrefix(
			r.Header.Get("Authorization"),
			"Bearer ",
		)

		if token == "" {
			http.Error(w, unauthorizedMessage, http.StatusUnauthorized)

			return
		}

		next(w, r)
	}
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

// handleRun godoc
// @Summary Run a test
// @Description Trigger a specific test by ID. Can be sync or async depends of the test definition.
// @Tags Tests
// @Param id query string true "Test ID defined in tests definitions"
// @Produce json
// @Success 200 {object} domain.TestResult
// @Success 202 {object} map[string]string "Async mode: returns run_id and status"
// @Failure 401 {string} string "Unauthorized"
// @Failure 405 {string} string "Method not allowed"
// @Failure 404 {string} string "Test ID not found"
// @Router /run [post]
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

// handleResultByID godoc
// @Summary Get a specific test result
// @Description Get a test result by its unique Run ID
// @Tags Results
// @Param run_id path string true "Run ID"
// @Produce json
// @Success 200 {object} domain.TestResult
// @Failure 401 {string} string "Unauthorized"
// @Failure 400 {string} string "Run ID required"
// @Failure 404 {string} string "Run not found"
// @Router /results/{run_id} [get]
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

// handleResults godoc
// @Summary List test results
// @Description Get the last 100 test results
// @Tags Results
// @Produce json
// @Success 200 {array} domain.TestResult
// @Failure 401 {string} string "Unauthorized"
// @Router /results [get]
func (s *Server) handleResults(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	list := make([]*domain.TestResult, 0, len(s.resultOrder))
	for _, id := range s.resultOrder {
		list = append(list, s.results[id])
	}
	s.mu.RUnlock()
	respondJSON(w, http.StatusOK, list)
}

// handleHealth godoc
// @Summary Check service health
// @Description Get the current status of the service
// @Tags Health
// @Produce json
// @Success 200 {object} map[string]string
// @Router /health [get]
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleRunSequence godoc
// @Summary Run a sequence of tests
// @Description Execute an ordered list of test IDs sequentially. Each test completes before the next starts.
// @Tags Tests
// @Accept json
// @Produce json
// @Param rules body []string true "Ordered list of test IDs to execute"
// @Param test_delay query string false "Duration to wait between tests (e.g. '2s'). Not applied before the first test."
// @Param skip_fail_test query bool false "Stop the sequence after the first failed or errored test (default false)"
// @Success 200 {array} domain.TestResult
// @Failure 400 {string} string "Invalid body or parameters"
// @Failure 401 {string} string "Unauthorized"
// @Failure 404 {string} string "Test ID not found"
// @Failure 405 {string} string "Method not allowed"
// @Router /run-sequence [post]
func (s *Server) handleRunSequence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)

		return
	}

	var rules []string
	if err := json.NewDecoder(r.Body).Decode(&rules); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)

		return
	}

	if len(rules) == 0 {
		http.Error(w, "rules must not be empty", http.StatusBadRequest)

		return
	}

	var delay time.Duration

	if raw := r.URL.Query().Get("test_delay"); raw != "" {
		var err error

		delay, err = time.ParseDuration(raw)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid test_delay: %v", err), http.StatusBadRequest)

			return
		}
	}

	skipFailTest := r.URL.Query().Get("skip_fail_test") == "true"

	defs := make([]domain.TestDefinition, 0, len(rules))

	for _, id := range rules {
		def, ok := s.tests[id]
		if !ok {
			http.Error(w, fmt.Sprintf("test %q not found", id), http.StatusNotFound)

			return
		}

		defs = append(defs, def)
	}

	ctx := context.WithoutCancel(r.Context())
	results := s.orchestrator.RunSequence(ctx, defs, services.SequenceConfig{
		Delay:        delay,
		SkipFailTest: skipFailTest,
	})

	for _, res := range results {
		s.storeResult(res)
	}

	respondJSON(w, http.StatusOK, results)
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
