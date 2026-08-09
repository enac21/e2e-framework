package notifier

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"e2e-framework/internal/core/domain"
)

func testResult(status string, vars map[string]string) *domain.TestResult {
	return &domain.TestResult{
		TestID:      "t1",
		RunID:       "run-1",
		Status:      status,
		Error:       "boom",
		TriggerVars: vars,
	}
}

func TestNotify_SkipsNonFailedResults(t *testing.T) {
	for _, status := range []string{domain.StatusPassed, domain.StatusSkipped} {
		var hits int
		var mu sync.Mutex

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			hits++
		}))
		defer srv.Close()

		cfg := domain.OnFailureConfig{Calls: []domain.CallAction{{URL: srv.URL}}}
		if err := NewHTTPNotifier().Notify(context.Background(), cfg, testResult(status, nil)); err != nil {
			t.Fatalf("status %s: unexpected error: %v", status, err)
		}

		mu.Lock()
		got := hits
		mu.Unlock()

		if got != 0 {
			t.Errorf("status %s: expected no calls, got %d", status, got)
		}
	}
}

func TestNotify_RunsOnErrorStatus(t *testing.T) {
	var hit bool
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		hit = true
	}))
	defer srv.Close()

	cfg := domain.OnFailureConfig{Calls: []domain.CallAction{{URL: srv.URL}}}
	if err := NewHTTPNotifier().Notify(context.Background(), cfg, testResult(domain.StatusError, nil)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if !hit {
		t.Error("expected the call to run on status error")
	}
}

func TestNotify_NoCallsIsNoOp(t *testing.T) {
	if err := NewHTTPNotifier().Notify(context.Background(), domain.OnFailureConfig{}, testResult(domain.StatusFailed, nil)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

type capturedCall struct {
	method      string
	path        string
	contentType string
	body        []byte
}

func TestNotify_SequentialCallsWithSubstitution(t *testing.T) {
	var mu sync.Mutex
	var calls []capturedCall

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, capturedCall{
			method:      r.Method,
			path:        r.URL.Path,
			contentType: r.Header.Get("Content-Type"),
			body:        body,
		})

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := domain.OnFailureConfig{
		Calls: []domain.CallAction{
			{URL: srv.URL + "/one", Body: map[string]any{"test_id": "{{test_id}}", "txn": "{{transaction_id}}"}},
			{Method: "PUT", URL: srv.URL + "/two", Body: map[string]any{"error": "{{error}}"}},
		},
	}

	if err := NewHTTPNotifier().Notify(context.Background(), cfg, testResult(domain.StatusFailed, map[string]string{"transaction_id": "txn-42"})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}

	if calls[0].path != "/one" || calls[1].path != "/two" {
		t.Errorf("expected sequential order /one,/two, got %v and %v", calls[0].path, calls[1].path)
	}

	var first map[string]string
	if err := json.Unmarshal(calls[0].body, &first); err != nil {
		t.Fatalf("invalid first body: %v", err)
	}

	if first["test_id"] != "t1" || first["txn"] != "txn-42" {
		t.Errorf("unexpected first body: %v", first)
	}

	var second map[string]string
	if err := json.Unmarshal(calls[1].body, &second); err != nil {
		t.Fatalf("invalid second body: %v", err)
	}

	if second["error"] != "boom" {
		t.Errorf("unexpected second body: %v", second)
	}
}

func TestNotify_DefaultMethodAndContentType(t *testing.T) {
	var mu sync.Mutex
	var got capturedCall

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		mu.Lock()
		defer mu.Unlock()
		got = capturedCall{method: r.Method, contentType: r.Header.Get("Content-Type"), body: body}
	}))
	defer srv.Close()

	cfg := domain.OnFailureConfig{Calls: []domain.CallAction{{URL: srv.URL, Body: map[string]any{"a": "b"}}}}
	if err := NewHTTPNotifier().Notify(context.Background(), cfg, testResult(domain.StatusFailed, nil)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if got.method != http.MethodPost {
		t.Errorf("expected default method POST, got %s", got.method)
	}

	if got.contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", got.contentType)
	}
}

func TestNotify_SkipsCallWithUnresolvedVariable(t *testing.T) {
	var mu sync.Mutex
	firstHits := 0
	secondHit := false

	srvFirst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		firstHits++
	}))
	defer srvFirst.Close()

	srvSecond := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		secondHit = true
	}))
	defer srvSecond.Close()

	cfg := domain.OnFailureConfig{
		Calls: []domain.CallAction{
			{URL: srvFirst.URL + "/skip", Body: map[string]any{"txn": "{{transaction_id}}"}},
			{URL: srvSecond.URL + "/ok"},
		},
	}

	// transaction_id was never extracted, so the first call must be skipped.
	if err := NewHTTPNotifier().Notify(context.Background(), cfg, testResult(domain.StatusFailed, nil)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if firstHits != 0 {
		t.Errorf("call with unresolved variable should be skipped, got %d hits", firstHits)
	}

	if !secondHit {
		t.Error("expected the second call to still run after a skip")
	}
}

func TestNotify_ContinuesAfterFailedCall(t *testing.T) {
	secondHit := false
	var mu sync.Mutex

	srvFirst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srvFirst.Close()

	srvSecond := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		secondHit = true
	}))
	defer srvSecond.Close()

	cfg := domain.OnFailureConfig{
		Calls: []domain.CallAction{
			{URL: srvFirst.URL},
			{URL: srvSecond.URL},
		},
	}

	if err := NewHTTPNotifier().Notify(context.Background(), cfg, testResult(domain.StatusFailed, nil)); err == nil {
		t.Fatal("expected aggregated error, got nil")
	}

	mu.Lock()
	defer mu.Unlock()

	if !secondHit {
		t.Error("expected the second call to run after the first failed")
	}
}

func TestNotify_ExpectedStatusMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	cfg := domain.OnFailureConfig{Calls: []domain.CallAction{{URL: srv.URL, ExpectedStatus: http.StatusOK}}}
	if err := NewHTTPNotifier().Notify(context.Background(), cfg, testResult(domain.StatusFailed, nil)); err == nil {
		t.Fatal("expected error on expected_status mismatch, got nil")
	}
}

func TestNotify_ServerErrorIsReturned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := domain.OnFailureConfig{Calls: []domain.CallAction{{URL: srv.URL}}}
	if err := NewHTTPNotifier().Notify(context.Background(), cfg, testResult(domain.StatusFailed, nil)); err == nil {
		t.Fatal("expected error on 5xx response, got nil")
	}
}

func TestNotify_DelayBefore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	start := time.Now()
	cfg := domain.OnFailureConfig{Calls: []domain.CallAction{{URL: srv.URL, DelayBefore: 5 * time.Millisecond}}}
	if err := NewHTTPNotifier().Notify(context.Background(), cfg, testResult(domain.StatusFailed, nil)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if elapsed := time.Since(start); elapsed < 5*time.Millisecond {
		t.Errorf("expected delay before the call, got elapsed %v", elapsed)
	}
}
