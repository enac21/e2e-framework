// Package store provides the pluggable message-store backends.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/pkg/config"
	"e2e-framework/internal/pkg/logger"

	_ "modernc.org/sqlite"
)

const (
	sqliteSchemaSQL = `
CREATE TABLE IF NOT EXISTS e2e_messages (
    run_id        TEXT NOT NULL,
    receiver_type TEXT NOT NULL,
    payload       BLOB NOT NULL,
    expires_at    INTEGER NOT NULL,
    PRIMARY KEY (run_id, receiver_type)
);

CREATE TABLE IF NOT EXISTS e2e_reservations (
    channel    TEXT NOT NULL,
    recipient  TEXT NOT NULL,
    run_id     TEXT NOT NULL,
    expires_at INTEGER NOT NULL,
    PRIMARY KEY (channel, recipient)
);
`

	sqlitePurgeMessagesSQL     = `DELETE FROM e2e_messages WHERE expires_at <= ?`
	sqlitePurgeReservationsSQL = `DELETE FROM e2e_reservations WHERE expires_at <= ?`

	sqliteDepositSQL = `
INSERT INTO e2e_messages (run_id, receiver_type, payload, expires_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (run_id, receiver_type)
DO UPDATE SET payload = excluded.payload, expires_at = excluded.expires_at
`

	sqliteClaimSQL = `
SELECT payload FROM e2e_messages
WHERE run_id = ? AND receiver_type = ? AND expires_at > ?
`

	sqliteReserveSQL = `
INSERT INTO e2e_reservations (channel, recipient, run_id, expires_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (channel, recipient) DO NOTHING
`

	sqliteSelectReservationSQL = `SELECT run_id FROM e2e_reservations WHERE channel = ? AND recipient = ?`

	sqliteReleaseSQL = `DELETE FROM e2e_reservations WHERE channel = ? AND recipient = ?`

	sqliteDeleteMessageSQL = `DELETE FROM e2e_messages WHERE run_id = ? AND receiver_type = ?`
)

type SQLiteStore struct {
	db     *sql.DB
	ttl    time.Duration
	logger *logger.Logger
}

func NewSQLiteStore(cfg config.SQLiteStoreConfig) (*SQLiteStore, error) {
	if len(cfg.Path) == 0 {
		return nil, fmt.Errorf("%w: sqlite mode requires a path", domain.ErrConfiguration)
	}

	dsn := "file:" + cfg.Path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to open sqlite store: %v", domain.ErrInternal, err)
	}

	// SQLite allows a single writer; capping the pool also avoids SQLITE_BUSY
	// under concurrent access.
	db.SetMaxOpenConns(1)

	ctx := context.Background()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("%w: failed to connect to sqlite store: %v", domain.ErrInternal, err)
	}

	s := &SQLiteStore{
		db:     db,
		ttl:    cfg.TTL,
		logger: logger.New("store").With(logger.AttrStoreType, "sqlite"),
	}

	if err := s.init(ctx); err != nil {
		_ = db.Close()

		return nil, err
	}

	return s, nil
}

func (s *SQLiteStore) Claim(ctx context.Context, runID string, receiverType string) (*domain.Message, error) {
	_ = s.purgeExpired(ctx)

	var data []byte

	err := s.db.QueryRowContext(ctx, sqliteClaimSQL, runID, receiverType, time.Now().UnixNano()).Scan(&data)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("%w: failed to claim message: %v", domain.ErrInternal, err)
	}

	var msg domain.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("%w: failed to deserialize message: %v", domain.ErrInternal, err)
	}

	return &msg, nil
}

func (s *SQLiteStore) Deposit(ctx context.Context, msg *domain.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("%w: failed to serialize message: %v", domain.ErrInternal, err)
	}

	_ = s.purgeExpired(ctx)

	expiresAt := time.Now().Add(s.ttl).UnixNano()

	if _, err := s.db.ExecContext(ctx, sqliteDepositSQL, msg.RunID, msg.ReceiverType, data, expiresAt); err != nil {
		return fmt.Errorf("%w: failed to deposit message: %v", domain.ErrInternal, err)
	}

	return nil
}

func (s *SQLiteStore) Reserve(ctx context.Context, channel string, recipient string, runID string) error {
	_ = s.purgeExpired(ctx)

	expiresAt := time.Now().Add(reservationTTL).UnixNano()

	result, err := s.db.ExecContext(ctx, sqliteReserveSQL, channel, recipient, runID, expiresAt)
	if err != nil {
		return fmt.Errorf("%w: failed to reserve %s:%s: %v", domain.ErrInternal, channel, recipient, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%w: failed to reserve %s:%s: %v", domain.ErrInternal, channel, recipient, err)
	}

	if rows == 0 {
		var existingRunID string
		_ = s.db.QueryRowContext(ctx, sqliteSelectReservationSQL, channel, recipient).Scan(&existingRunID)

		return fmt.Errorf(
			"%w: recipient %s:%s already reserved by run %s",
			domain.ErrInternal, channel, recipient, existingRunID,
		)
	}

	return nil
}

func (s *SQLiteStore) Release(ctx context.Context, channel string, recipient string) error {
	if _, err := s.db.ExecContext(ctx, sqliteReleaseSQL, channel, recipient); err != nil {
		return fmt.Errorf("%w: failed to release %s:%s: %v", domain.ErrInternal, channel, recipient, err)
	}

	return nil
}

func (s *SQLiteStore) Delete(ctx context.Context, runID string, receiverType string) error {
	if _, err := s.db.ExecContext(ctx, sqliteDeleteMessageSQL, runID, receiverType); err != nil {
		return fmt.Errorf("%w: failed to delete message: %v", domain.ErrInternal, err)
	}

	return nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) init(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, sqliteSchemaSQL); err != nil {
		return fmt.Errorf("%w: failed to init sqlite schema: %v", domain.ErrInternal, err)
	}

	return nil
}

func (s *SQLiteStore) purgeExpired(ctx context.Context) error {
	now := time.Now().UnixNano()

	messagesResult, err := s.db.ExecContext(ctx, sqlitePurgeMessagesSQL, now)
	if err != nil {
		return err
	}

	reservationsResult, err := s.db.ExecContext(ctx, sqlitePurgeReservationsSQL, now)
	if err != nil {
		return err
	}

	messagesDeleted, err := messagesResult.RowsAffected()
	if err != nil {
		return err
	}

	reservationsDeleted, err := reservationsResult.RowsAffected()
	if err != nil {
		return err
	}

	deleted := messagesDeleted + reservationsDeleted
	if deleted > 0 && s.logger != nil {
		s.logger.Debug().Int("deleted_count", int(deleted)).Msg("expired results purged")
	}

	return nil
}
