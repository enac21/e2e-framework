package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
	"e2e-framework/internal/core/services"
	"e2e-framework/internal/pkg/logger"

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
	logger       *logger.Logger
	mu           sync.RWMutex
}

type Config struct {
	Port       int
	AuthEnable bool
	JWTSecret  string
	Resolver   ports.GroupResolver
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
		logger:       logger.New("api"),
	}

	s.registerRoute(mux, http.MethodGet, "/health", http.HandlerFunc(s.handleHealth))
	s.registerRoute(mux, http.MethodPost, "/run", s.authMiddleware(s.handleRun))
	s.registerRoute(mux, http.MethodPost, "/run-sequence", s.authMiddleware(s.handleRunSequence))
	s.registerRoute(mux, http.MethodGet, "/results", s.authMiddleware(s.handleResults))
	s.registerRoute(mux, http.MethodGet, "/results/{run_id}", s.authMiddleware(s.handleResultByID))
	s.registerRoute(mux, http.MethodGet, "/swagger/*", s.authMiddleware(httpSwagger.WrapHandler.ServeHTTP))

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: s.loggingMiddleware(mux),
	}

	return s
}

// registerRoute wires a handler and logs a line per route with the exact
// registration pattern.
func (s *Server) registerRoute(mux *http.ServeMux, method, pattern string, handler http.Handler) {
	mux.Handle(pattern, handler)

	s.logger.Debug().
		Str(logger.AttrMethod, method).
		Str(logger.AttrPath, pattern).
		Msg("http route registered")
}

// loggingMiddleware records one structured line per HTTP request with
// method, path, status and duration. /health is logged at Debug to keep
// probe noise out of production streams.
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		level := slog.LevelInfo
		if r.URL.Path == "/health" {
			level = slog.LevelDebug
		}

		s.logger.Event(level).
			Str(logger.AttrMethod, r.Method).
			Str(logger.AttrPath, r.URL.Path).
			Int(logger.AttrRespStatus, rec.status).
			Duration(logger.AttrDurationMS, time.Since(start)).
			Msg("http request completed")
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
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
			s.logger.Warn().
				Str(logger.AttrMethod, r.Method).
				Str(logger.AttrPath, r.URL.Path).
				Msg("unauthorized request")

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

		s.logger.Debug().
			Str(logger.AttrRunID, oldest).
			Int("max_results", maxStoredResults).
			Msg("result evicted from cache")
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
		s.logger.Warn().
			Str(logger.AttrTestID, id).
			Str(logger.AttrPath, r.URL.Path).
			Msg("test not found")

		http.Error(w, fmt.Sprintf("test %q not found", id), http.StatusNotFound)

		return
	}

	runID, resultCh := s.orchestrator.RunTest(ctx, def, logger.ApiFlow)

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
// @Description Execute an ordered list of test IDs sequentially, or run a configured test group. The body is a plain JSON array of test IDs; alternatively pass test_group as a query param to expand a configured group.
// @Tags Tests
// @Accept json
// @Produce json
// @Param test_group query string false "Name of a configured test group to run instead of an explicit list"
// @Param body body []string false "Ordered list of test IDs (not used when test_group is set)"
// @Param test_delay query string false "Duration to wait between tests (e.g. '2s'). Not applied before the first test."
// @Param skip_fail_test query bool false "Stop the sequence after the first failed or errored test (default false)"
// @Success 200 {array} domain.TestResult
// @Failure 400 {string} string "Invalid body or parameters"
// @Failure 401 {string} string "Unauthorized"
// @Failure 404 {string} string "Test ID or test group not found"
// @Failure 405 {string} string "Method not allowed"
// @Router /run-sequence [post]
func (s *Server) handleRunSequence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)

		return
	}

	rules := make([]string, 0)
	var group *ports.TestGroup

	if groupName := r.URL.Query().Get("test_group"); groupName != "" {
		if s.cfg.Resolver == nil {
			s.logger.Warn().
				Str("group", groupName).
				Str(logger.AttrPath, r.URL.Path).
				Msg("test group not found")

			http.Error(w, fmt.Sprintf("test group %q not found", groupName), http.StatusNotFound)

			return
		}

		resolved, ok := s.cfg.Resolver.Resolve(groupName)
		if !ok {
			s.logger.Warn().
				Str("group", groupName).
				Str(logger.AttrPath, r.URL.Path).
				Msg("test group not found")

			http.Error(w, fmt.Sprintf("test group %q not found", groupName), http.StatusNotFound)

			return
		}

		group = &resolved
		rules = resolved.Tests
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)

		return
	}

	if trimmed := strings.TrimSpace(string(body)); trimmed != "" {
		if group != nil {
			http.Error(w, "test_group and body test list are mutually exclusive", http.StatusBadRequest)

			return
		}

		if err := json.Unmarshal([]byte(trimmed), &rules); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)

			return
		}
	}

	if len(rules) == 0 {
		http.Error(w, "rules must not be empty", http.StatusBadRequest)

		return
	}

	var delay time.Duration
	delaySet := false

	if raw := r.URL.Query().Get("test_delay"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid test_delay: %v", err), http.StatusBadRequest)

			return
		}

		delay = parsed
		delaySet = true
	}

	var skipFailTest bool
	skipSet := false

	if raw := r.URL.Query().Get("skip_fail_test"); raw != "" {
		skipFailTest = raw == "true"
		skipSet = true
	}

	if group != nil {
		if !delaySet {
			delay = group.TestDelay
		}

		if !skipSet {
			skipFailTest = group.SkipFailTest
		}
	}

	defs := make([]domain.TestDefinition, 0, len(rules))

	for _, id := range rules {
		def, ok := s.tests[id]
		if !ok {
			s.logger.Warn().
				Str(logger.AttrTestID, id).
				Str(logger.AttrPath, r.URL.Path).
				Msg("test not found")

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
