package webhook

import (
	"context"
	"fmt"
	"time"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type WebhookReceiver struct {
	store ports.Store
	runID string
}

func NewWebhookReceiver(store ports.Store) *WebhookReceiver {
	return &WebhookReceiver{
		store: store,
	}
}

func (r *WebhookReceiver) Start(ctx context.Context, runID string) error {
	r.runID = runID
	return nil
}

func (r *WebhookReceiver) Collect(ctx context.Context) (*domain.Message, error) {
	if r.runID == "" {
		return nil, fmt.Errorf("receiver not started")
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for webhook message: %w", ctx.Err())
		case <-ticker.C:
			msg, err := r.store.Claim(ctx, r.runID, "webhook")
			if err != nil {
				return nil, fmt.Errorf("failed to claim message from store: %w", err)
			}
			if msg != nil {
				return msg, nil
			}
		}
	}
}

func (r *WebhookReceiver) Stop() error {
	return nil
}
