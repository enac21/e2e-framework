package receiver

import (
	"fmt"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type ReceiverAssertionFactory func(cfg domain.AssertionConfig) (ports.Assertion, error)

type ReceiverAssertionRegistry struct {
	factories map[string]ReceiverAssertionFactory
}

func NewReceiverAssertionRegistry() *ReceiverAssertionRegistry {
	return &ReceiverAssertionRegistry{
		factories: make(map[string]ReceiverAssertionFactory),
	}
}

func (r *ReceiverAssertionRegistry) Register(typeName string, factory ReceiverAssertionFactory) {
	r.factories[typeName] = factory
}

func (r *ReceiverAssertionRegistry) Create(cfg domain.AssertionConfig) (ports.Assertion, error) {
	if factory, ok := r.factories[cfg.Type]; ok {
		return factory(cfg)
	}

	return nil, fmt.Errorf("%w: unknown assertion type: %q", domain.ErrConfiguration, cfg.Type)
}
