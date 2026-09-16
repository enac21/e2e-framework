package assertion

import (
	"fmt"
	"strings"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type lengthAssertion struct {
	field string
	value string
}

func NewLengthAssertion(cfg domain.AssertionConfig) (ports.Assertion, error) {
	return &lengthAssertion{field: cfg.Field, value: cfg.Value}, nil
}

func (a *lengthAssertion) Assert(msg *domain.Message) error {
	field := strings.ToLower(a.field)
	lenKey := field + ".__len__"
	actualLen, exists := msg.Fields[lenKey]
	if !exists {
		return fmt.Errorf("%w: field %q: field is not an array or does not exist", domain.ErrValidation, a.field)
	}
	if actualLen != a.value {
		return fmt.Errorf("%w: field %q: expected length %s, got %s", domain.ErrValidation, a.field, a.value, actualLen)
	}
	return nil
}
