package store

import (
	"context"
	"testing"

	"e2e-framework/internal/core/domain"
)

func TestDisabledStore_NoOps(t *testing.T) {
	ctx := context.Background()
	s := NewDisabledStore()

	msg := &domain.Message{RunID: "r", ReceiverType: "request"}

	if err := s.Deposit(ctx, msg); err != nil {
		t.Fatalf("Deposit: %v", err)
	}

	got, err := s.Claim(ctx, "r", "request")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}

	if got != nil {
		t.Fatalf("expected nil from Claim, got %+v", got)
	}

	if err := s.Reserve(ctx, "sms", "555", "run"); err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	if err := s.Release(ctx, "sms", "555"); err != nil {
		t.Fatalf("Release: %v", err)
	}

	if err := s.Delete(ctx, "r", "request"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
