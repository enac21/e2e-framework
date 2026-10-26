package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"e2e-framework/internal/pkg/logger"
)

// newTestServer builds a minimal Server whose logger writes JSON records
// into a buffer, so the middleware output can be asserted.
func newTestServer(t *testing.T) (*Server, *bytes.Buffer) {
	t.Helper()

	buf := &bytes.Buffer{}
	lg := logger.NewWithHandler(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	return &Server{logger: lg}, buf
}

// records parses one JSON log line per call and returns them as maps.
func records(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	var out []map[string]any
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	for dec.More() {
		var rec map[string]any
		if err := dec.Decode(&rec); err != nil {
			t.Fatalf("decode log record: %v", err)
		}
		out = append(out, rec)
	}
	return out
}

func TestLoggingMiddlewareLogsRequestAtInfo(t *testing.T) {
	s, buf := newTestServer(t)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	handler := s.loggingMiddleware(next)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/run", nil))

	recs := records(t, buf)
	if len(recs) != 1 {
		t.Fatalf("expected 1 log record, got %d", len(recs))
	}

	rec := recs[0]
	if rec["msg"] != "http request completed" {
		t.Errorf("msg = %v", rec["msg"])
	}
	if rec["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", rec["level"])
	}
	if rec["method"] != "POST" {
		t.Errorf("method = %v, want POST", rec["method"])
	}
	if rec["path"] != "/run" {
		t.Errorf("path = %v, want /run", rec["path"])
	}
	if rec["resp_status"] != float64(http.StatusNotFound) {
		t.Errorf("resp_status = %v, want 404", rec["resp_status"])
	}
	if _, ok := rec["duration_ms"].(float64); !ok {
		t.Errorf("duration_ms = %v, want a number", rec["duration_ms"])
	}
}

func TestLoggingMiddlewareLogsHealthAtDebug(t *testing.T) {
	s, buf := newTestServer(t)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := s.loggingMiddleware(next)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))

	recs := records(t, buf)
	if len(recs) != 1 {
		t.Fatalf("expected 1 log record, got %d", len(recs))
	}

	if recs[0]["level"] != "DEBUG" {
		t.Errorf("level = %v, want DEBUG for /health", recs[0]["level"])
	}
	if recs[0]["resp_status"] != float64(http.StatusOK) {
		t.Errorf("resp_status = %v, want 200", recs[0]["resp_status"])
	}
}

func TestStatusRecorderDefaultsToOK(t *testing.T) {
	rec := &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	if rec.status != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.status)
	}

	rec.WriteHeader(http.StatusTeapot)
	if rec.status != http.StatusTeapot {
		t.Errorf("status = %d, want 418", rec.status)
	}
}
