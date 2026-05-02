package receiver

import (
	"fmt"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type ReceiverFactory func() ports.Receiver

type ReceiverRegistry struct {
	factories map[string]ReceiverFactory
}

func NewReceiverRegistry() *ReceiverRegistry {
	return &ReceiverRegistry{
		factories: make(map[string]ReceiverFactory),
	}
}

func (r *ReceiverRegistry) Register(typeName string, factory ReceiverFactory) {
	r.factories[typeName] = factory
}

func (r *ReceiverRegistry) Create(typeName string) (ports.Receiver, error) {
	if factory, ok := r.factories[typeName]; ok {
		return factory(), nil
	}

	return nil, fmt.Errorf("%w: unknown receiver type: %q", domain.ErrConfiguration, typeName)
}
