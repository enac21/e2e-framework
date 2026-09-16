package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type Ingestor struct {
	store ports.Store
}

func NewIngestor(store ports.Store) (*Ingestor, error) {
	if store == nil {
		return nil, fmt.Errorf("%w: store is required", domain.ErrConfiguration)
	}
	return &Ingestor{store: store}, nil
}

func (i *Ingestor) Ingest(ctx context.Context, msg *domain.Message) error {
	if msg == nil {
		return fmt.Errorf("%w: message is required", domain.ErrValidation)
	}
	if strings.TrimSpace(msg.RunID) == "" || msg.RunID == "unknown" {
		return fmt.Errorf("%w: run_id missing", domain.ErrValidation)
	}
	if msg.ReceivedAt.IsZero() {
		msg.ReceivedAt = time.Now()
	}
	if err := i.store.Deposit(ctx, msg); err != nil {
		return fmt.Errorf("%w: %v", domain.ErrInternal, err)
	}
	return nil
}
