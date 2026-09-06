//go:build integration

package store

import (
	"context"
	"os"
	"testing"
	"time"

	"e2e-framework/internal/core/domain"
)

func newPostgresStoreForTest(t *testing.T) *PostgresStore {
	t.Helper()

	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://e2e:e2e@localhost:5432/e2e"
	}

	s, err := NewPostgresStore(PostgresStoreConfig{DSN: dsn, TTL: time.Minute})
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	return s
}

func TestPostgresStore_DepositClaimRoundtrip(t *testing.T) {
	ctx := context.Background()
	s := newPostgresStoreForTest(t)

	msg := &domain.Message{
		RunID:        "pg-run-1",
		ReceiverType: "request",
		Fields:       map[string]string{"body": "hello"},
	}

	if err := s.Deposit(ctx, msg); err != nil {
		t.Fatalf("Deposit: %v", err)
	}

	got, err := s.Claim(ctx, "pg-run-1", "request")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}

	if got == nil || got.RunID != "pg-run-1" {
		t.Fatalf("unexpected message: %+v", got)
	}
}

func TestPostgresStore_ClaimMissing(t *testing.T) {
	ctx := context.Background()
	s := newPostgresStoreForTest(t)

	got, err := s.Claim(ctx, "missing", "request")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}

	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestPostgresStore_ReserveConflictAndRelease(t *testing.T) {
	ctx := context.Background()
	s := newPostgresStoreForTest(t)

	if err := s.Reserve(ctx, "sms", "pg555", "run-a"); err != nil {
		t.Fatalf("first Reserve: %v", err)
	}

	err := s.Reserve(ctx, "sms", "pg555", "run-b")
	if err == nil {
		t.Fatal("expected conflict error")
	}

	if err := s.Release(ctx, "sms", "pg555"); err != nil {
		t.Fatalf("Release: %v", err)
	}

	if err := s.Reserve(ctx, "sms", "pg555", "run-c"); err != nil {
		t.Fatalf("Reserve after release: %v", err)
	}
}

func TestPostgresStore_Delete(t *testing.T) {
	ctx := context.Background()
	s := newPostgresStoreForTest(t)

	if err := s.Deposit(ctx, &domain.Message{RunID: "pg-del", ReceiverType: "request"}); err != nil {
		t.Fatalf("Deposit: %v", err)
	}

	if err := s.Delete(ctx, "pg-del", "request"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, _ := s.Claim(ctx, "pg-del", "request")
	if got != nil {
		t.Fatalf("expected nil after Delete, got %+v", got)
	}
}
