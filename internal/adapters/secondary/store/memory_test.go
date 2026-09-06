package store

import (
	"context"
	"testing"
	"time"

	"e2e-framework/internal/core/domain"
)

func newMemStoreWithTTL(ttl time.Duration) *MemoryStore {
	return NewMemoryStore(MemoryStoreConfig{TTL: ttl})
}

func TestMemoryStore_DepositClaimRoundtrip(t *testing.T) {
	ctx := context.Background()
	s := newMemStoreWithTTL(time.Minute)

	msg := &domain.Message{
		RunID:        "run-1",
		ReceiverType: "request",
		Fields:       map[string]string{"body": "hello"},
	}

	if err := s.Deposit(ctx, msg); err != nil {
		t.Fatalf("Deposit: %v", err)
	}

	got, err := s.Claim(ctx, "run-1", "request")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}

	if got == nil {
		t.Fatal("expected message, got nil")
	}

	if got.RunID != "run-1" || got.ReceiverType != "request" {
		t.Errorf("unexpected message: %+v", got)
	}

	if got.Fields["body"] != "hello" {
		t.Errorf("expected body field, got %+v", got.Fields)
	}
}

func TestMemoryStore_ClaimMissing(t *testing.T) {
	ctx := context.Background()
	s := newMemStoreWithTTL(time.Minute)

	got, err := s.Claim(ctx, "nope", "request")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}

	if got != nil {
		t.Fatalf("expected nil for missing key, got %+v", got)
	}
}

func TestMemoryStore_DepositOverwrite(t *testing.T) {
	ctx := context.Background()
	s := newMemStoreWithTTL(time.Minute)

	first := &domain.Message{RunID: "r", ReceiverType: "request", Fields: map[string]string{"v": "1"}}
	second := &domain.Message{RunID: "r", ReceiverType: "request", Fields: map[string]string{"v": "2"}}

	if err := s.Deposit(ctx, first); err != nil {
		t.Fatalf("Deposit first: %v", err)
	}

	if err := s.Deposit(ctx, second); err != nil {
		t.Fatalf("Deposit second: %v", err)
	}

	got, _ := s.Claim(ctx, "r", "request")
	if got.Fields["v"] != "2" {
		t.Errorf("expected updated value, got %+v", got.Fields)
	}
}

func TestMemoryStore_ClaimExpired(t *testing.T) {
	ctx := context.Background()
	s := newMemStoreWithTTL(-time.Second)

	if err := s.Deposit(ctx, &domain.Message{RunID: "r", ReceiverType: "request"}); err != nil {
		t.Fatalf("Deposit: %v", err)
	}

	got, err := s.Claim(ctx, "r", "request")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}

	if got != nil {
		t.Fatalf("expected nil for expired entry, got %+v", got)
	}
}

func TestMemoryStore_ReserveConflictAndRelease(t *testing.T) {
	ctx := context.Background()
	s := newMemStoreWithTTL(time.Minute)

	if err := s.Reserve(ctx, "sms", "555", "run-a"); err != nil {
		t.Fatalf("first Reserve: %v", err)
	}

	err := s.Reserve(ctx, "sms", "555", "run-b")
	if err == nil {
		t.Fatal("expected conflict error on second Reserve")
	}

	if err := s.Release(ctx, "sms", "555"); err != nil {
		t.Fatalf("Release: %v", err)
	}

	if err := s.Reserve(ctx, "sms", "555", "run-c"); err != nil {
		t.Fatalf("Reserve after release: %v", err)
	}
}

func TestMemoryStore_ReserveExpired(t *testing.T) {
	ctx := context.Background()
	s := newMemStoreWithTTL(time.Minute)

	// Reservation TTL is fixed (reservationTTL); simulate expiry by inserting
	// an already-expired reservation directly.
	s.mu.Lock()
	s.reservations["sms:555"] = memReservation{runID: "old", expiresAt: time.Now().Add(-time.Second)}
	s.mu.Unlock()

	if err := s.Reserve(ctx, "sms", "555", "run-new"); err != nil {
		t.Fatalf("Reserve after expiry should succeed, got: %v", err)
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	ctx := context.Background()
	s := newMemStoreWithTTL(time.Minute)

	if err := s.Deposit(ctx, &domain.Message{RunID: "r", ReceiverType: "request"}); err != nil {
		t.Fatalf("Deposit: %v", err)
	}

	if err := s.Delete(ctx, "r", "request"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, _ := s.Claim(ctx, "r", "request")
	if got != nil {
		t.Fatalf("expected nil after Delete, got %+v", got)
	}
}
