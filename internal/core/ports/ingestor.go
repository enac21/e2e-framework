package ports

import (
	"context"

	"e2e-framework/internal/core/domain"
)

//go:generate go run -mod=mod go.uber.org/mock/mockgen -source=ingestor.go -destination=mocks/mock_ingestor.go -package=mocks

type MessageIngestor interface {
	Ingest(ctx context.Context, msg *domain.Message) error
}
