package receiver

import (
	"fmt"

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
	factory, exists := r.factories[typeName]
	if !exists {
		return nil, fmt.Errorf("unknown receiver type: %q", typeName)
	}
	return factory(), nil
}
