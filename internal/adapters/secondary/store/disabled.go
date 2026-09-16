package store

import (
	"context"

	"e2e-framework/internal/core/domain"
)

type DisabledStore struct{}

func NewDisabledStore() *DisabledStore {
	return &DisabledStore{}
}

func (s *DisabledStore) Deposit(ctx context.Context, msg *domain.Message) error {
	return nil
}

func (s *DisabledStore) Claim(ctx context.Context, runID string, receiverType string) (*domain.Message, error) {
	return nil, nil
}

func (s *DisabledStore) Reserve(ctx context.Context, channel string, recipient string, runID string) error {
	return nil
}

func (s *DisabledStore) Release(ctx context.Context, channel string, recipient string) error {
	return nil
}

func (s *DisabledStore) Delete(ctx context.Context, runID string, receiverType string) error {
	return nil
}

func (s *DisabledStore) Close() error {
	return nil
}
