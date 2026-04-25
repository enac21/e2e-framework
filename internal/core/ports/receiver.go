package ports

import (
	"context"

	"e2e-framework/internal/core/domain"
)

type Receiver interface {
	Start(ctx context.Context, runID string) error
	Collect(ctx context.Context) (*domain.Message, error)
	Stop() error
}
