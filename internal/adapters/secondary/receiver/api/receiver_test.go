package api_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"e2e-framework/internal/adapters/secondary/receiver/api"
	"e2e-framework/internal/core/domain"
)

func newReceiver(t *testing.T, cfg domain.ReceiverConfig) *api.APIPollingReceiver {
	t.Helper()

	r, err := api.NewAPIPollingReceiver(cfg, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	return r
}

func collectWithTimeout(t *testing.T, r *api.APIPollingReceiver, d time.Duration) (*domain.Message, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()

	return r.Collect(ctx)
}

func TestNewAPIPollingReceiver_MissingURL(t *testing.T) {
	_, err := api.NewAPIPollingReceiver(domain.ReceiverConfig{}, nil, nil)

	if !errors.Is(err, domain.ErrConfiguration) {
		t.Fatalf("expected ErrConfiguration, got %v", err)
	}
}

func TestCollect_ReturnsMessageOnFirstSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"paid","data":{"id":"p-1"}}`))
	}))
	defer srv.Close()

	cfg := domain.ReceiverConfig{
		Type:           domain.APIReceiverType,
		URL:            srv.URL + "/orders/1",
		Interval:       time.Millisecond,
		ExpectedStatus: 200,
		ResponseAsserts: []domain.AssertionConfig{
			{Type: "equals", Field: "status", Value: "paid"},
		},
	}

	r := newReceiver(t, cfg)

	if err := r.Start(context.Background(), "run-1", map[string]string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg, err := collectWithTimeout(t, r, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg == nil {
		t.Fatal("expected non-nil message")
	}

	if msg.ReceiverType != domain.APIReceiverType {
		t.Errorf("expected receiver type %q, got %q", domain.APIReceiverType, msg.ReceiverType)
	}

	if msg.RunID != "run-1" {
		t.Errorf("expected runID %q, got %q", "run-1", msg.RunID)
	}

	if msg.Fields["status"] != "paid" {
		t.Errorf("expected flattened status %q, got %q", "paid", msg.Fields["status"])
	}

	if msg.Fields["data.id"] != "p-1" {
		t.Errorf("expected flattened data.id %q, got %q", "p-1", msg.Fields["data.id"])
	}
}

func TestCollect_RetriesStatusMismatchThenSucceeds(t *testing.T) {
	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))

			return
		}

		_, _ = w.Write([]byte(`{"status":"paid"}`))
	}))
	defer srv.Close()

	cfg := domain.ReceiverConfig{
		Type:           domain.APIReceiverType,
		URL:            srv.URL,
		Interval:       time.Millisecond,
		ExpectedStatus: 200,
		ResponseAsserts: []domain.AssertionConfig{
			{Type: "equals", Field: "status", Value: "paid"},
		},
	}

	r := newReceiver(t, cfg)

	if err := r.Start(context.Background(), "run-retry", map[string]string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg, err := collectWithTimeout(t, r, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg == nil {
		t.Fatal("expected non-nil message")
	}

	if calls.Load() != 2 {
		t.Errorf("expected 2 attempts, got %d", calls.Load())
	}
}

func TestCollect_RetriesThenTimesOutWithErrTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"pending"}`))
	}))
	defer srv.Close()

	cfg := domain.ReceiverConfig{
		Type:           domain.APIReceiverType,
		URL:            srv.URL,
		Interval:       time.Millisecond,
		ExpectedStatus: 200,
		ResponseAsserts: []domain.AssertionConfig{
			{Type: "equals", Field: "status", Value: "paid"},
		},
	}

	r := newReceiver(t, cfg)

	if err := r.Start(context.Background(), "run-timeout", map[string]string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := collectWithTimeout(t, r, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, domain.ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
}

func TestCollect_SubstitutesVarsInURLHeadersBody(t *testing.T) {
	var gotURL, gotAuth string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		gotAuth = r.Header.Get("Authorization")

		b, _ := io.ReadAll(r.Body)
		gotBody = b

		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	cfg := domain.ReceiverConfig{
		Type:   domain.APIReceiverType,
		Method: http.MethodPost,
		URL:    srv.URL + "/orders/{{order_id}}",
		Headers: map[string]string{
			"Authorization": "Bearer {{token}}",
			"Content-Type":  "application/json",
		},
		Body: map[string]any{
			"order_id": "{{order_id}}",
			"comment":  "{{comment}}",
		},
		Interval:       time.Millisecond,
		ExpectedStatus: 200,
	}

	r := newReceiver(t, cfg)

	if err := r.Start(context.Background(), "run-vars", map[string]string{
		"order_id": "42",
		"token":    "abc",
		"comment":  "hello",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg, err := collectWithTimeout(t, r, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg == nil {
		t.Fatal("expected non-nil message")
	}

	if !strings.Contains(gotURL, "/orders/42") {
		t.Errorf("expected url to contain order_id substitution, got %q", gotURL)
	}

	if gotAuth != "Bearer abc" {
		t.Errorf("expected Authorization %q, got %q", "Bearer abc", gotAuth)
	}

	if !strings.Contains(string(gotBody), "hello") {
		t.Errorf("expected body to contain comment substitution, got %q", string(gotBody))
	}

	if !strings.Contains(string(gotBody), "42") {
		t.Errorf("expected body to contain order_id substitution, got %q", string(gotBody))
	}
}

func TestCollect_NotStarted(t *testing.T) {
	r, err := api.NewAPIPollingReceiver(domain.ReceiverConfig{URL: "http://example.com"}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = r.Collect(context.Background())

	if !errors.Is(err, domain.ErrConfiguration) {
		t.Fatalf("expected ErrConfiguration, got %v", err)
	}
}

func TestCollect_MessageStyleFieldsAvailableForAssertions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"body":"thanks for your order"}`))
	}))
	defer srv.Close()

	cfg := domain.ReceiverConfig{
		Type:           domain.APIReceiverType,
		URL:            srv.URL,
		Interval:       time.Millisecond,
		ExpectedStatus: 201,
	}

	r := newReceiver(t, cfg)

	if err := r.Start(context.Background(), "run-msg", map[string]string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg, err := collectWithTimeout(t, r, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.Fields["body"] != "thanks for your order" {
		t.Errorf("expected flattened body field, got %q", msg.Fields["body"])
	}
}
