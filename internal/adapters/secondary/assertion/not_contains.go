package assertion

import (
	"fmt"
	"strings"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type NotContainsAssertion struct {
	field string
	value string
}

func NewNotContainsAssertion(cfg domain.AssertionConfig) (ports.Assertion, error) {
	return &NotContainsAssertion{
		field: cfg.Field,
		value: cfg.Value,
	}, nil
}

func (a *NotContainsAssertion) Assert(msg *domain.Message) error {
	actual := msg.Fields[a.field]
	if strings.Contains(actual, a.value) {
		return fmt.Errorf("%w: field %q: expected NOT to contain %q, but it does", domain.ErrValidation, a.field, a.value)
	}

	return nil
}
