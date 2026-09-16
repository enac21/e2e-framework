package assertion

import (
	"fmt"
	"strings"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type equalsAssertion struct {
	field string
	value string
}

func NewEqualsAssertion(cfg domain.AssertionConfig) (ports.Assertion, error) {
	return &equalsAssertion{field: cfg.Field, value: cfg.Value}, nil
}

func (a *equalsAssertion) Assert(msg *domain.Message) error {
	field := strings.ToLower(a.field)
	actual := msg.Fields[field]
	if actual != a.value {
		return fmt.Errorf("%w: field %q: expected %q, got %q", domain.ErrValidation, a.field, a.value, actual)
	}
	return nil
}
