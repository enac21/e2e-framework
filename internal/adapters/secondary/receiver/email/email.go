package email

import (
	"context"
	"fmt"
	"time"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type EmailReceiver struct {
	store ports.Store
	runID string
}

func NewEmailReceiver(store ports.Store) *EmailReceiver {
	return &EmailReceiver{
		store: store,
	}
}

func (r *EmailReceiver) Start(ctx context.Context, runID string) error {
	r.runID = runID

	return nil
}

func (r *EmailReceiver) Collect(ctx context.Context) (*domain.Message, error) {
	if r.runID == "" {
		return nil, fmt.Errorf("%w: receiver not started", domain.ErrConfiguration)
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: timeout waiting for email message: %v", domain.ErrTimeout, ctx.Err())
		case <-ticker.C:
			msg, err := r.store.Claim(ctx, r.runID, "email")
			if err != nil {
				return nil, fmt.Errorf("%w: failed to claim message from store: %v", domain.ErrInternal, err)
			}

			if msg != nil {
				return msg, nil
			}
		}
	}
}

func (r *EmailReceiver) Stop() error {
	return nil
}
