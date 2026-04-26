package ports

import (
	"context"

	"e2e-framework/internal/core/domain"
)

type Store interface {
	Deposit(ctx context.Context, msg *domain.Message) error
	Claim(ctx context.Context, runID string, receiverType string) (*domain.Message, error)
	Reserve(ctx context.Context, channel string, recipient string, runID string) error
	Release(ctx context.Context, channel string, recipient string) error
	Delete(ctx context.Context, runID string, receiverType string) error
	Close() error
}
