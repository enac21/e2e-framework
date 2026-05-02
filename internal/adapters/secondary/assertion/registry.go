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
	if factory, ok := r.factories[cfg.Type]; ok {
		return factory(cfg)
	}

	return nil, fmt.Errorf("%w: unknown assertion type: %q", domain.ErrConfiguration, cfg.Type)
}
