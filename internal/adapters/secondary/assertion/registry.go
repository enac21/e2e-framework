package assertion

import (
	"fmt"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type AssertionFactory func(cfg domain.AssertionConfig) (ports.Assertion, error)

type AssertionRegistry struct {
	factories map[string]AssertionFactory
}

func NewAssertionRegistry() *AssertionRegistry {
	return &AssertionRegistry{
		factories: make(map[string]AssertionFactory),
	}
}

func (r *AssertionRegistry) Register(typeName string, factory AssertionFactory) {
	r.factories[typeName] = factory
}

func (r *AssertionRegistry) Create(cfg domain.AssertionConfig) (ports.Assertion, error) {
	factory, exists := r.factories[cfg.Type]
	if !exists {
		return nil, fmt.Errorf("unknown assertion type: %q", cfg.Type)
	}
	return factory(cfg)
}
