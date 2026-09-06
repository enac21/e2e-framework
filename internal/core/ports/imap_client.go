package ports

import (
	"context"

	"e2e-framework/internal/core/domain"
)

//go:generate go run -mod=mod go.uber.org/mock/mockgen -source=imap_client.go -destination=mocks/mock_imap_client.go -package=mocks

type IMAPClient interface {
	Connect() error
	SearchByRunID(ctx context.Context, runID string) (*domain.Message, error)
	Disconnect() error
}
