package push

import (
	"context"
	"fmt"
	"time"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type PushReceiver struct {
	store ports.Store
	runID string
}

func NewPushReceiver(store ports.Store) ports.Receiver {
	return &PushReceiver{
		store: store,
	}
}

func (r *PushReceiver) Start(ctx context.Context, runID string) error {
	r.runID = runID
	return nil
}

func (r *PushReceiver) Collect(ctx context.Context) (*domain.Message, error) {
	if r.runID == "" {
		return nil, fmt.Errorf("receiver not started")
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for push message: %w", ctx.Err())
		case <-ticker.C:
			msg, err := r.store.Claim(ctx, r.runID, "push")
			if err != nil {
				return nil, fmt.Errorf("failed to claim message from store: %w", err)
			}
			if msg != nil {
				return msg, nil
			}
		}
	}
}

func (r *PushReceiver) Stop() error {
	return nil
}
