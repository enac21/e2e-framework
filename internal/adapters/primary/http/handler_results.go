package http

import (
	"net/http"
)

func (s *Server) handleResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	res := s.results
	s.mu.RUnlock()

	respondJSON(w, http.StatusOK, res)
}
