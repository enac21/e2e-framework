package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"e2e-framework/internal/core/domain"

	"github.com/redis/go-redis/v9"
)

const e2eTestKey = "e2e-test:%s:%s"
const reservationTTL = 1 * time.Hour

type RedisStoreConfig struct {
	URL string
	TTL time.Duration
}

type RedisStore struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisStore(cfg RedisStoreConfig) (*RedisStore, error) {
	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid redis URL: %v", domain.ErrConfiguration, err)
	}

	return &RedisStore{
		client: redis.NewClient(opts),
		ttl:    cfg.TTL,
	}, nil
}

func (s *RedisStore) Deposit(ctx context.Context, msg *domain.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("%w: failed to serialize message: %v", domain.ErrInternal, err)
	}

	key := fmt.Sprintf(e2eTestKey, msg.RunID, msg.ReceiverType)

	return s.client.Set(ctx, key, data, s.ttl).Err()
}

func (s *RedisStore) Claim(ctx context.Context, runID string, receiverType string) (*domain.Message, error) {
	key := fmt.Sprintf(e2eTestKey, runID, receiverType)

	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}

		return nil, fmt.Errorf("%w: failed to claim message: %v", domain.ErrInternal, err)
	}

	var msg domain.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("%w: failed to deserialize message: %v", domain.ErrInternal, err)
	}

	return &msg, nil
}

func (s *RedisStore) Reserve(ctx context.Context, channel string, recipient string, runID string) error {
	key := fmt.Sprintf("store:reservations:%s:%s", channel, recipient)

	err := s.client.SetArgs(ctx, key, runID, redis.SetArgs{Mode: "NX", TTL: reservationTTL}).Err()
	if err == redis.Nil {
		existingRunID, _ := s.client.Get(ctx, key).Result()

		return fmt.Errorf("%w: recipient %s:%s already reserved by run %s", domain.ErrInternal, channel, recipient, existingRunID)
	} else if err != nil {
		return fmt.Errorf("%w: failed to reserve %s:%s: %v", domain.ErrInternal, channel, recipient, err)
	}

	return nil
}

func (s *RedisStore) Release(ctx context.Context, channel string, recipient string) error {
	key := fmt.Sprintf("store:reservations:%s:%s", channel, recipient)

	return s.client.Del(ctx, key).Err()
}

func (s *RedisStore) Delete(ctx context.Context, runID string, receiverType string) error {
	key := fmt.Sprintf(e2eTestKey, runID, receiverType)

	return s.client.Del(ctx, key).Err()
}

func (s *RedisStore) Close() error {
	return s.client.Close()
}
