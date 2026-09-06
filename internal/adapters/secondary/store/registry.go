package store

import (
	"fmt"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
	"e2e-framework/internal/pkg/config"
)

type StoreFactory func(cfg config.StoreConfig) (ports.Store, error)

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

func (r *StoreRegistry) Create(cfg config.StoreConfig) (ports.Store, error) {
	if cfg.Type == "" {
		return nil, fmt.Errorf("%w: store type not configured", domain.ErrConfiguration)
	}

	factory, ok := r.factories[cfg.Type]
	if !ok {
		return nil, fmt.Errorf("%w: unknown store type: %q", domain.ErrConfiguration, cfg.Type)
	}

	return factory(cfg)
}
