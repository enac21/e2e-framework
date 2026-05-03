package ports

import (
	"context"

	"e2e-framework/internal/core/domain"
)

type IMAPClient interface {
	Connect() error
	SearchByRunID(ctx context.Context, runID string) (*domain.Message, error)
	Disconnect() error
}
