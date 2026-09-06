package ports

import (
	"context"

	"e2e-framework/internal/core/domain"
)

//go:generate go run -mod=mod go.uber.org/mock/mockgen -source=receiver.go -destination=mocks/mock_receiver.go -package=mocks

type Receiver interface {
	Start(ctx context.Context, runID string, vars map[string]string) error
	Collect(ctx context.Context) (*domain.Message, error)
	Stop() error
}
