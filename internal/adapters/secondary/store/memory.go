package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"e2e-framework/internal/core/domain"
)

type MemoryStoreConfig struct {
	TTL time.Duration
}

type memEntry struct {
	payload   []byte
	expiresAt time.Time
}

type memReservation struct {
	runID     string
	expiresAt time.Time
}

// MemoryStore implements ports.Store using in-process, mutex-protected maps.
// It is intended for single-process environments and tests that must not
// depend on an external database.
type MemoryStore struct {
	mu           sync.Mutex
	messages     map[string]memEntry
	reservations map[string]memReservation
	ttl          time.Duration
}

func NewMemoryStore(cfg MemoryStoreConfig) *MemoryStore {
	return &MemoryStore{
		messages:     make(map[string]memEntry),
		reservations: make(map[string]memReservation),
		ttl:          cfg.TTL,
	}
}

func (s *MemoryStore) Deposit(ctx context.Context, msg *domain.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("%w: failed to serialize message: %v", domain.ErrInternal, err)
	}

	key := fmt.Sprintf("%s:%s", msg.RunID, msg.ReceiverType)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.messages[key] = memEntry{payload: data, expiresAt: time.Now().Add(s.ttl)}

	return nil
}

func (s *MemoryStore) Claim(ctx context.Context, runID string, receiverType string) (*domain.Message, error) {
	key := fmt.Sprintf("%s:%s", runID, receiverType)

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.messages[key]
	if !ok || entry.expiresAt.Before(time.Now()) {
		return nil, nil
	}

	var msg domain.Message
	if err := json.Unmarshal(entry.payload, &msg); err != nil {
		return nil, fmt.Errorf("%w: failed to deserialize message: %v", domain.ErrInternal, err)
	}

	return &msg, nil
}

func (s *MemoryStore) Reserve(ctx context.Context, channel string, recipient string, runID string) error {
	key := fmt.Sprintf("%s:%s", channel, recipient)

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.reservations[key]; ok && existing.expiresAt.After(time.Now()) {
		return fmt.Errorf("%w: recipient %s:%s already reserved by run %s", domain.ErrInternal, channel, recipient, existing.runID)
	}

	s.reservations[key] = memReservation{runID: runID, expiresAt: time.Now().Add(reservationTTL)}

	return nil
}

func (s *MemoryStore) Release(ctx context.Context, channel string, recipient string) error {
	key := fmt.Sprintf("%s:%s", channel, recipient)

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.reservations, key)

	return nil
}

func (s *MemoryStore) Delete(ctx context.Context, runID string, receiverType string) error {
	key := fmt.Sprintf("%s:%s", runID, receiverType)

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.messages, key)

	return nil
}

func (s *MemoryStore) Close() error {
	return nil
}
