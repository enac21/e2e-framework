package webhook

import (
	"context"
	"fmt"
	"time"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
	"e2e-framework/internal/pkg/logger"
)

type WebhookReceiver struct {
	store     ports.Store
	logger    *logger.Logger
	runID     string
	startedAt time.Time
}

func NewWebhookReceiver(store ports.Store) *WebhookReceiver {
	return &WebhookReceiver{
		store:  store,
		logger: logger.New("receiver").With(logger.AttrReceiver, domain.WebhookReceiverType),
	}
}

func (r *WebhookReceiver) Start(ctx context.Context, runID string, vars map[string]string) error {
	r.runID = runID
	r.startedAt = time.Now()

	r.logger.Debug().
		Str(logger.AttrRunID, runID).
		Msg("waiting for webhook message")

	return nil
}

func (r *WebhookReceiver) Collect(ctx context.Context) (*domain.Message, error) {
	if r.runID == "" {
		return nil, fmt.Errorf("%w: receiver not started", domain.ErrConfiguration)
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: timeout waiting for webhook message: %v", domain.ErrTimeout, ctx.Err())
		case <-ticker.C:
			msg, err := r.store.Claim(ctx, r.runID, domain.WebhookReceiverType)
			if err != nil {
				return nil, fmt.Errorf("%w: failed to claim message from store: %v", domain.ErrInternal, err)
			}

			if msg != nil {
				waited := time.Duration(0)
				if !r.startedAt.IsZero() {
					waited = time.Since(r.startedAt)
				}

				r.logger.Info().
					Str(logger.AttrRunID, r.runID).
					Duration("waited_ms", waited).
					Msg("webhook message received")

				return msg, nil
			}
		}
	}
}

func (r *WebhookReceiver) Stop() error {
	return nil
}
