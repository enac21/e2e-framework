package assertion

import (
	"fmt"
	"strings"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type intAssertion struct {
	field string
	value string
	typ   string
}

func NewIntEqAssertion(cfg domain.AssertionConfig) (ports.Assertion, error) {
	return &intAssertion{field: cfg.Field, value: cfg.Value, typ: "int_eq"}, nil
}

func NewIntGtAssertion(cfg domain.AssertionConfig) (ports.Assertion, error) {
	return &intAssertion{field: cfg.Field, value: cfg.Value, typ: "int_gt"}, nil
}

func NewIntGteAssertion(cfg domain.AssertionConfig) (ports.Assertion, error) {
	return &intAssertion{field: cfg.Field, value: cfg.Value, typ: "int_gte"}, nil
}

func NewIntLtAssertion(cfg domain.AssertionConfig) (ports.Assertion, error) {
	return &intAssertion{field: cfg.Field, value: cfg.Value, typ: "int_lt"}, nil
}

func NewIntLteAssertion(cfg domain.AssertionConfig) (ports.Assertion, error) {
	return &intAssertion{field: cfg.Field, value: cfg.Value, typ: "int_lte"}, nil
}

func (a *intAssertion) Assert(msg *domain.Message) error {
	field := strings.ToLower(a.field)
	actualNum, actualInt := ParseInt(msg.Fields[field])
	valueNum, valueInt := ParseInt(a.value)
	if !actualInt {
		return fmt.Errorf("%w: field %q: %s requires an integer field value, got %q", domain.ErrValidation, a.field, a.typ, msg.Fields[field])
	}
	if !valueInt {
		return fmt.Errorf("%w: field %q: %s requires an integer expected value, got %q", domain.ErrValidation, a.field, a.typ, a.value)
	}
	if passes, symbol := CompareInts(a.typ, actualNum, valueNum); !passes {
		return fmt.Errorf("%w: field %q: %s failed: got %d, want %s %d", domain.ErrValidation, a.field, a.typ, actualNum, symbol, valueNum)
	}
	return nil
}
