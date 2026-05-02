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

func NewPushReceiver(store ports.Store) *PushReceiver {
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
		return nil, fmt.Errorf("%w: receiver not started", domain.ErrConfiguration)
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: timeout waiting for push message: %v", domain.ErrTimeout, ctx.Err())
		case <-ticker.C:
			msg, err := r.store.Claim(ctx, r.runID, "push")
			if err != nil {
				return nil, fmt.Errorf("%w: failed to claim message from store: %v", domain.ErrInternal, err)
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
