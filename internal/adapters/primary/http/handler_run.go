package http

import (
	"context"
	"net/http"
)

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	testID := r.URL.Query().Get("id")
	if testID == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "missing id query parameter"})
		return
	}

	def, exists := s.tests[testID]
	if !exists {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "test not found"})
		return
	}

	// For MVP, run synchronously and return the result.
	// In production, we'd return a 202 Accepted and the run_id to poll.
	result := s.orchestrator.RunTest(context.Background(), def)
	s.addResult(result)

	respondJSON(w, http.StatusOK, result)
}
