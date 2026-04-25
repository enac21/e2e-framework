package assertion

import (
	"fmt"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type EqualsAssertion struct {
	field string
	value string
}

func NewEqualsAssertion(cfg domain.AssertionConfig) (ports.Assertion, error) {
	return &EqualsAssertion{
		field: cfg.Field,
		value: cfg.Value,
	}, nil
}

func (a *EqualsAssertion) Assert(msg *domain.Message) error {
	actual := msg.Fields[a.field]
	if actual != a.value {
		return fmt.Errorf("field %q: expected %q, got %q", a.field, a.value, actual)
	}
	return nil
}
