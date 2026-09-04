package trigger

import (
	"fmt"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type TriggerFactory func(options map[string]string) (ports.Trigger, error)

type TriggerRegistry struct {
	factories map[string]TriggerFactory
}

func NewTriggerRegistry() *TriggerRegistry {
	return &TriggerRegistry{
		factories: make(map[string]TriggerFactory),
	}
}

func (r *TriggerRegistry) Register(typeName string, factory TriggerFactory) {
	r.factories[typeName] = factory
}

func (r *TriggerRegistry) Create(typeName string, options map[string]string) (ports.Trigger, error) {
	if factory, ok := r.factories[typeName]; ok {
		return factory(options)
	}

	return nil, fmt.Errorf("%w: unknown trigger type: %q", domain.ErrConfiguration, typeName)
}
