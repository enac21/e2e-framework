package assertion

import (
	"fmt"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type PresentAssertion struct {
	field string
}

func NewPresentAssertion(cfg domain.AssertionConfig) (ports.Assertion, error) {
	return &PresentAssertion{
		field: cfg.Field,
	}, nil
}

func (a *PresentAssertion) Assert(msg *domain.Message) error {
	actual, exists := msg.Fields[a.field]
	if !exists || actual == "" {
		return fmt.Errorf("%w: field %q: expected to be present, but was empty or missing", domain.ErrValidation, a.field)
	}

	return nil
}
