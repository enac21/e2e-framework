// Package store provides the pluggable message store backends behind the
// ports.Store contract. A StoreRegistry selects the concrete backend at
// wiring time based on the configured store type.
package store

import (
	"fmt"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

// StoreConfig carries every backend's configuration. Only the fields matching
// the selected store type are consumed by its factory.
type StoreConfig struct {
	Type     string
	Redis    RedisStoreConfig
	Postgres PostgresStoreConfig
	Memory   MemoryStoreConfig
}

// StoreFactory builds a concrete ports.Store from a StoreConfig.
type StoreFactory func(cfg StoreConfig) (ports.Store, error)

// StoreRegistry holds the set of registered store backends.
type StoreRegistry struct {
	factories map[string]StoreFactory
}

func NewStoreRegistry() *StoreRegistry {
	return &StoreRegistry{
		factories: make(map[string]StoreFactory),
	}
}

func (r *StoreRegistry) Register(typeName string, factory StoreFactory) {
	r.factories[typeName] = factory
}

func (r *StoreRegistry) Create(typeName string, cfg StoreConfig) (ports.Store, error) {
	if factory, ok := r.factories[typeName]; ok {
		return factory(cfg)
	}

	return nil, fmt.Errorf("%w: unknown store type: %q", domain.ErrConfiguration, typeName)
}
