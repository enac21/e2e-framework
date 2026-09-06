package store

import (
	"context"

	"e2e-framework/internal/core/domain"
)

type NoopStore struct{}

func NewNoopStore() *NoopStore {
	return &NoopStore{}
}

func (s *NoopStore) Deposit(ctx context.Context, msg *domain.Message) error {
	return nil
}

func (s *NoopStore) Claim(ctx context.Context, runID string, receiverType string) (*domain.Message, error) {
	return nil, nil
}

func (s *NoopStore) Reserve(ctx context.Context, channel string, recipient string, runID string) error {
	return nil
}

func (s *NoopStore) Release(ctx context.Context, channel string, recipient string) error {
	return nil
}

func (s *NoopStore) Delete(ctx context.Context, runID string, receiverType string) error {
	return nil
}

func (s *NoopStore) Close() error {
	return nil
}
