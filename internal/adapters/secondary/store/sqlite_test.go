package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/pkg/config"
)

func TestSQLiteStore_NewSQLiteStore(t *testing.T) {
	t.Run("missing path", func(t *testing.T) {
		_, err := NewSQLiteStore(config.SQLiteStoreConfig{})
		if err == nil {
			t.Fatal("expected error for missing path")
		}

		if !errors.Is(err, domain.ErrConfiguration) {
			t.Errorf("expected ErrConfiguration, got %v", err)
		}
	})

	t.Run("creates file and schema", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "e2e.db")

		s, err := NewSQLiteStore(config.SQLiteStoreConfig{Path: path, TTL: time.Minute})
		if err != nil {
			t.Fatalf("NewSQLiteStore: %v", err)
		}

		t.Cleanup(func() { _ = s.Close() })

		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected database file to exist: %v", err)
		}

		const schemaCountSQL = `SELECT COUNT(*) FROM sqlite_master` +
			` WHERE type = 'table' AND name IN ('e2e_messages', 'e2e_reservations')`

		var count int
		if err := s.db.QueryRowContext(context.Background(), schemaCountSQL).Scan(&count); err != nil {
			t.Fatalf("query schema: %v", err)
		}

		if count != 2 {
			t.Errorf("expected 2 tables, got %d", count)
		}

		var journalMode string
		if err := s.db.QueryRowContext(context.Background(), "PRAGMA journal_mode").Scan(&journalMode); err != nil {
			t.Fatalf("query journal_mode: %v", err)
		}

		if journalMode != "wal" {
			t.Errorf("expected journal_mode wal, got %q", journalMode)
		}

		var busyTimeout int
		if err := s.db.QueryRowContext(context.Background(), "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
			t.Fatalf("query busy_timeout: %v", err)
		}

		if busyTimeout != 5000 {
			t.Errorf("expected busy_timeout 5000, got %d", busyTimeout)
		}
	})

	t.Run("reopens existing database", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "e2e.db")

		first, err := NewSQLiteStore(config.SQLiteStoreConfig{Path: path, TTL: time.Minute})
		if err != nil {
			t.Fatalf("NewSQLiteStore first: %v", err)
		}

		msg := &domain.Message{RunID: "run-1", ReceiverType: "webhook", Fields: map[string]string{"body": "hello"}}

		if err := first.Deposit(context.Background(), msg); err != nil {
			t.Fatalf("Deposit: %v", err)
		}

		if err := first.Reserve(context.Background(), "sms", "555", "run-1"); err != nil {
			t.Fatalf("Reserve: %v", err)
		}

		if err := first.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		second, err := NewSQLiteStore(config.SQLiteStoreConfig{Path: path, TTL: time.Minute})
		if err != nil {
			t.Fatalf("NewSQLiteStore second: %v", err)
		}

		t.Cleanup(func() { _ = second.Close() })

		got, err := second.Claim(context.Background(), "run-1", "webhook")
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}

		if got == nil || got.Fields["body"] != "hello" {
			t.Fatalf("expected persisted message after restart, got %+v", got)
		}
	})

	t.Run("drops expired rows after restart", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "e2e.db")

		first, err := NewSQLiteStore(config.SQLiteStoreConfig{Path: path, TTL: -time.Second})
		if err != nil {
			t.Fatalf("NewSQLiteStore first: %v", err)
		}

		if err := first.Deposit(context.Background(), &domain.Message{RunID: "run-1", ReceiverType: "webhook"}); err != nil {
			t.Fatalf("Deposit: %v", err)
		}

		if err := first.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		second, err := NewSQLiteStore(config.SQLiteStoreConfig{Path: path, TTL: -time.Second})
		if err != nil {
			t.Fatalf("NewSQLiteStore second: %v", err)
		}

		t.Cleanup(func() { _ = second.Close() })

		got, err := second.Claim(context.Background(), "run-1", "webhook")
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}

		if got != nil {
			t.Fatalf("expected nil after restart for expired row, got %+v", got)
		}

		var count int
		if err := second.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM e2e_messages").Scan(&count); err != nil {
			t.Fatalf("query message count: %v", err)
		}

		if count != 0 {
			t.Errorf("expected expired rows purged after restart, got %d", count)
		}
	})
}

func TestSQLiteStore_Claim(t *testing.T) {
	t.Run("missing returns nil", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "e2e.db")

		s, err := NewSQLiteStore(config.SQLiteStoreConfig{Path: path, TTL: time.Minute})
		if err != nil {
			t.Fatalf("NewSQLiteStore: %v", err)
		}

		t.Cleanup(func() { _ = s.Close() })

		got, err := s.Claim(context.Background(), "missing", "webhook")
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}

		if got != nil {
			t.Fatalf("expected nil for missing key, got %+v", got)
		}
	})

	t.Run("expired returns nil", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "e2e.db")

		s, err := NewSQLiteStore(config.SQLiteStoreConfig{Path: path, TTL: -time.Second})
		if err != nil {
			t.Fatalf("NewSQLiteStore: %v", err)
		}

		t.Cleanup(func() { _ = s.Close() })

		msg := &domain.Message{RunID: "r", ReceiverType: "webhook"}
		if err := s.Deposit(context.Background(), msg); err != nil {
			t.Fatalf("Deposit: %v", err)
		}

		got, err := s.Claim(context.Background(), "r", "webhook")
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}

		if got != nil {
			t.Fatalf("expected nil for expired entry, got %+v", got)
		}
	})

	t.Run("expired row is filtered by the claim query", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "e2e.db")

		s, err := NewSQLiteStore(config.SQLiteStoreConfig{Path: path, TTL: time.Minute})
		if err != nil {
			t.Fatalf("NewSQLiteStore: %v", err)
		}

		t.Cleanup(func() { _ = s.Close() })

		expiredAt := time.Now().Add(-time.Hour).UnixNano()

		_, err = s.db.ExecContext(
			context.Background(),
			"INSERT INTO e2e_messages (run_id, receiver_type, payload, expires_at) VALUES (?, ?, ?, ?)",
			"r", "webhook", []byte(`{}`), expiredAt,
		)
		if err != nil {
			t.Fatalf("insert expired row: %v", err)
		}

		var payload []byte

		err = s.db.QueryRowContext(
			context.Background(), sqliteClaimSQL, "r", "webhook", time.Now().UnixNano(),
		).Scan(&payload)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expected sql.ErrNoRows from claim query, got %v", err)
		}
	})
}

func TestSQLiteStore_Deposit(t *testing.T) {
	t.Run("roundtrip stores message", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "e2e.db")

		s, err := NewSQLiteStore(config.SQLiteStoreConfig{Path: path, TTL: time.Minute})
		if err != nil {
			t.Fatalf("NewSQLiteStore: %v", err)
		}

		t.Cleanup(func() { _ = s.Close() })

		msg := &domain.Message{
			RunID:        "run-1",
			ReceiverType: "webhook",
			Fields:       map[string]string{"body": "hello"},
		}

		if err := s.Deposit(context.Background(), msg); err != nil {
			t.Fatalf("Deposit: %v", err)
		}

		got, err := s.Claim(context.Background(), "run-1", "webhook")
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}

		if got == nil {
			t.Fatal("expected message, got nil")
		}

		if got.RunID != "run-1" || got.ReceiverType != "webhook" {
			t.Errorf("unexpected message: %+v", got)
		}

		if got.Fields["body"] != "hello" {
			t.Errorf("expected body field, got %+v", got.Fields)
		}
	})

	t.Run("overwrites existing message", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "e2e.db")

		s, err := NewSQLiteStore(config.SQLiteStoreConfig{Path: path, TTL: time.Minute})
		if err != nil {
			t.Fatalf("NewSQLiteStore: %v", err)
		}

		t.Cleanup(func() { _ = s.Close() })

		first := &domain.Message{RunID: "r", ReceiverType: "webhook", Fields: map[string]string{"v": "1"}}
		second := &domain.Message{RunID: "r", ReceiverType: "webhook", Fields: map[string]string{"v": "2"}}

		if err := s.Deposit(context.Background(), first); err != nil {
			t.Fatalf("Deposit first: %v", err)
		}

		if err := s.Deposit(context.Background(), second); err != nil {
			t.Fatalf("Deposit second: %v", err)
		}

		got, err := s.Claim(context.Background(), "r", "webhook")
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}

		if got == nil || got.Fields["v"] != "2" {
			t.Fatalf("expected updated value, got %+v", got)
		}
	})
}

func TestSQLiteStore_Reserve(t *testing.T) {
	t.Run("reserves recipient", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "e2e.db")

		s, err := NewSQLiteStore(config.SQLiteStoreConfig{Path: path, TTL: time.Minute})
		if err != nil {
			t.Fatalf("NewSQLiteStore: %v", err)
		}

		t.Cleanup(func() { _ = s.Close() })

		if err := s.Reserve(context.Background(), "sms", "555", "run-a"); err != nil {
			t.Fatalf("Reserve: %v", err)
		}
	})

	t.Run("rejects conflict", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "e2e.db")

		s, err := NewSQLiteStore(config.SQLiteStoreConfig{Path: path, TTL: time.Minute})
		if err != nil {
			t.Fatalf("NewSQLiteStore: %v", err)
		}

		t.Cleanup(func() { _ = s.Close() })

		if err := s.Reserve(context.Background(), "sms", "555", "run-a"); err != nil {
			t.Fatalf("first Reserve: %v", err)
		}

		err = s.Reserve(context.Background(), "sms", "555", "run-b")
		if err == nil {
			t.Fatal("expected conflict error on second Reserve")
		}

		if !errors.Is(err, domain.ErrInternal) {
			t.Errorf("expected ErrInternal, got %v", err)
		}

		if !strings.Contains(err.Error(), "already reserved") {
			t.Errorf("expected conflict message, got %v", err)
		}

		if !strings.Contains(err.Error(), "run-a") {
			t.Errorf("expected conflict message to name run-a, got %v", err)
		}
	})

	t.Run("allows re-reserve after expiry", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "e2e.db")

		s, err := NewSQLiteStore(config.SQLiteStoreConfig{Path: path, TTL: time.Minute})
		if err != nil {
			t.Fatalf("NewSQLiteStore: %v", err)
		}

		t.Cleanup(func() { _ = s.Close() })

		if err := s.Reserve(context.Background(), "sms", "555", "run-a"); err != nil {
			t.Fatalf("first Reserve: %v", err)
		}

		if _, err := s.db.ExecContext(context.Background(), "UPDATE e2e_reservations SET expires_at = 0"); err != nil {
			t.Fatalf("expire reservation: %v", err)
		}

		if err := s.Reserve(context.Background(), "sms", "555", "run-b"); err != nil {
			t.Fatalf("Reserve after expiry: %v", err)
		}
	})
}

func TestSQLiteStore_Release(t *testing.T) {
	t.Run("allows re-reserve after release", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "e2e.db")

		s, err := NewSQLiteStore(config.SQLiteStoreConfig{Path: path, TTL: time.Minute})
		if err != nil {
			t.Fatalf("NewSQLiteStore: %v", err)
		}

		t.Cleanup(func() { _ = s.Close() })

		if err := s.Reserve(context.Background(), "sms", "555", "run-a"); err != nil {
			t.Fatalf("Reserve: %v", err)
		}

		if err := s.Release(context.Background(), "sms", "555"); err != nil {
			t.Fatalf("Release: %v", err)
		}

		if err := s.Reserve(context.Background(), "sms", "555", "run-b"); err != nil {
			t.Fatalf("Reserve after release: %v", err)
		}
	})
}

func TestSQLiteStore_Delete(t *testing.T) {
	t.Run("removes message", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "e2e.db")

		s, err := NewSQLiteStore(config.SQLiteStoreConfig{Path: path, TTL: time.Minute})
		if err != nil {
			t.Fatalf("NewSQLiteStore: %v", err)
		}

		t.Cleanup(func() { _ = s.Close() })

		if err := s.Deposit(context.Background(), &domain.Message{RunID: "r", ReceiverType: "webhook"}); err != nil {
			t.Fatalf("Deposit: %v", err)
		}

		if err := s.Delete(context.Background(), "r", "webhook"); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		got, err := s.Claim(context.Background(), "r", "webhook")
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}

		if got != nil {
			t.Fatalf("expected nil after Delete, got %+v", got)
		}
	})
}

func TestSQLiteStore_Close(t *testing.T) {
	t.Run("rejects deposit after close", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "e2e.db")

		s, err := NewSQLiteStore(config.SQLiteStoreConfig{Path: path, TTL: time.Minute})
		if err != nil {
			t.Fatalf("NewSQLiteStore: %v", err)
		}

		if err := s.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		err = s.Deposit(context.Background(), &domain.Message{RunID: "r", ReceiverType: "webhook"})
		if err == nil {
			t.Fatal("expected error depositing into a closed store")
		}
	})
}
