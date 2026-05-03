package request

import (
	"context"
	"fmt"
	"time"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type RequestReceiver struct {
	store ports.Store
	runID string
}

func NewRequestReceiver(store ports.Store) *RequestReceiver {
	return &RequestReceiver{
		store: store,
	}
}

func (r *RequestReceiver) Start(ctx context.Context, runID string) error {
	r.runID = runID

	return nil
}

func (r *RequestReceiver) Collect(ctx context.Context) (*domain.Message, error) {
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
			msg, err := r.store.Claim(ctx, r.runID, domain.RequestReceiverType)
			if err != nil {
				return nil, fmt.Errorf("%w: failed to claim message from store: %v", domain.ErrInternal, err)
			}

			if msg != nil {
				return msg, nil
			}
		}
	}
}

func (r *RequestReceiver) Stop() error {
	return nil
}
