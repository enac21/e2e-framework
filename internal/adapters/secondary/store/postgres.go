package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"e2e-framework/internal/core/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStoreConfig struct {
	DSN string
	TTL time.Duration
}

// PostgresStore implements ports.Store on top of a PostgreSQL database.
type PostgresStore struct {
	pool *pgxpool.Pool
	ttl  time.Duration
}

func NewPostgresStore(cfg PostgresStoreConfig) (*PostgresStore, error) {
	if len(cfg.DSN) == 0 {
		return nil, fmt.Errorf("%w: postgres mode requires a dsn", domain.ErrConfiguration)
	}

	pool, err := pgxpool.New(context.Background(), cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to connect to postgres store: %v", domain.ErrInternal, err)
	}

	s := &PostgresStore{pool: pool, ttl: cfg.TTL}

	if err := s.init(context.Background()); err != nil {
		pool.Close()

		return nil, err
	}

	return s, nil
}

func (s *PostgresStore) init(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS e2e_messages (
    run_id        TEXT NOT NULL,
    receiver_type TEXT NOT NULL,
    payload       JSONB NOT NULL,
    expires_at    TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (run_id, receiver_type)
);

CREATE TABLE IF NOT EXISTS e2e_reservations (
    channel    TEXT NOT NULL,
    recipient  TEXT NOT NULL,
    run_id     TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (channel, recipient)
);
`)
	if err != nil {
		return fmt.Errorf("%w: failed to init postgres schema: %v", domain.ErrInternal, err)
	}

	return nil
}

func (s *PostgresStore) purgeExpired(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM e2e_messages WHERE expires_at <= now()`); err != nil {
		return err
	}

	_, err := s.pool.Exec(ctx, `DELETE FROM e2e_reservations WHERE expires_at <= now()`)

	return err
}

func (s *PostgresStore) Deposit(ctx context.Context, msg *domain.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("%w: failed to serialize message: %v", domain.ErrInternal, err)
	}

	_ = s.purgeExpired(ctx)

	expiresAt := time.Now().Add(s.ttl)

	_, err = s.pool.Exec(ctx, `
INSERT INTO e2e_messages (run_id, receiver_type, payload, expires_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (run_id, receiver_type)
DO UPDATE SET payload = EXCLUDED.payload, expires_at = EXCLUDED.expires_at
`, msg.RunID, msg.ReceiverType, data, expiresAt)
	if err != nil {
		return fmt.Errorf("%w: failed to deposit message: %v", domain.ErrInternal, err)
	}

	return nil
}

func (s *PostgresStore) Claim(ctx context.Context, runID string, receiverType string) (*domain.Message, error) {
	_ = s.purgeExpired(ctx)

	var data []byte
	row := s.pool.QueryRow(ctx, `
SELECT payload FROM e2e_messages
WHERE run_id = $1 AND receiver_type = $2 AND expires_at > now()
`, runID, receiverType)

	err := row.Scan(&data)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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

func (s *PostgresStore) Reserve(ctx context.Context, channel string, recipient string, runID string) error {
	_ = s.purgeExpired(ctx)

	expiresAt := time.Now().Add(reservationTTL)

	ct, err := s.pool.Exec(ctx, `
INSERT INTO e2e_reservations (channel, recipient, run_id, expires_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (channel, recipient) DO NOTHING
`, channel, recipient, runID, expiresAt)
	if err != nil {
		return fmt.Errorf("%w: failed to reserve %s:%s: %v", domain.ErrInternal, channel, recipient, err)
	}

	if ct.RowsAffected() == 0 {
		var existingRunID string
		_ = s.pool.QueryRow(ctx, `
SELECT run_id FROM e2e_reservations WHERE channel = $1 AND recipient = $2
`, channel, recipient).Scan(&existingRunID)

		return fmt.Errorf("%w: recipient %s:%s already reserved by run %s", domain.ErrInternal, channel, recipient, existingRunID)
	}

	return nil
}

func (s *PostgresStore) Release(ctx context.Context, channel string, recipient string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM e2e_reservations WHERE channel = $1 AND recipient = $2`, channel, recipient)
	if err != nil {
		return fmt.Errorf("%w: failed to release %s:%s: %v", domain.ErrInternal, channel, recipient, err)
	}

	return nil
}

func (s *PostgresStore) Delete(ctx context.Context, runID string, receiverType string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM e2e_messages WHERE run_id = $1 AND receiver_type = $2`, runID, receiverType)
	if err != nil {
		return fmt.Errorf("%w: failed to delete message: %v", domain.ErrInternal, err)
	}

	return nil
}

func (s *PostgresStore) Close() error {
	s.pool.Close()

	return nil
}
