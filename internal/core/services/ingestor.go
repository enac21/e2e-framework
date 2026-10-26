package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
	"e2e-framework/internal/pkg/logger"
)

type Ingestor struct {
	store  ports.Store
	logger *logger.Logger
}

func NewIngestor(store ports.Store) (*Ingestor, error) {
	if store == nil {
		return nil, fmt.Errorf("%w: store is required", domain.ErrConfiguration)
	}

	return &Ingestor{
		store:  store,
		logger: logger.New("ingestor"),
	}, nil
}

func (i *Ingestor) Ingest(ctx context.Context, msg *domain.Message) error {
	if msg == nil {
		i.logger.Warn().
			Str(logger.AttrError, "message is required").
			Msg("message rejected")

		return fmt.Errorf("%w: message is required", domain.ErrValidation)
	}

	if strings.TrimSpace(msg.RunID) == "" || msg.RunID == "unknown" {
		i.logger.Warn().
			Str(logger.AttrRunID, msg.RunID).
			Str(logger.AttrError, "run_id missing").
			Msg("message rejected")

		return fmt.Errorf("%w: run_id missing", domain.ErrValidation)
	}

	if msg.ReceivedAt.IsZero() {
		msg.ReceivedAt = time.Now()
	}

	if err := i.store.Deposit(ctx, msg); err != nil {
		i.logger.Warn().
			Str(logger.AttrRunID, msg.RunID).
			Err(err).
			Msg("message rejected")

		return fmt.Errorf("%w: %v", domain.ErrInternal, err)
	}

	i.logger.Debug().
		Str(logger.AttrRunID, msg.RunID).
		Str(logger.AttrReceiver, msg.ReceiverType).
		Msg("message stored")

	return nil
}
