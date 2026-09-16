package assertion

import (
	"fmt"
	"strings"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type presentAssertion struct {
	field string
}

func NewPresentAssertion(cfg domain.AssertionConfig) (ports.Assertion, error) {
	return &presentAssertion{field: cfg.Field}, nil
}

func (a *presentAssertion) Assert(msg *domain.Message) error {
	field := strings.ToLower(a.field)
	actual, exists := msg.Fields[field]
	if !exists || actual == "" {
		return fmt.Errorf("%w: field %q: expected to be present, but was empty or missing", domain.ErrValidation, a.field)
	}
	return nil
}
