package assertion

import (
	"fmt"
	"strings"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type containsAssertion struct {
	field string
	value string
}

func NewContainsAssertion(cfg domain.AssertionConfig) (ports.Assertion, error) {
	return &containsAssertion{field: cfg.Field, value: cfg.Value}, nil
}

func (a *containsAssertion) Assert(msg *domain.Message) error {
	field := strings.ToLower(a.field)
	actual := msg.Fields[field]
	if !strings.Contains(actual, a.value) {
		return fmt.Errorf("%w: field %q: expected to contain %q, got %q", domain.ErrValidation, a.field, a.value, actual)
	}
	return nil
}

type notContainsAssertion struct {
	field string
	value string
}

func NewNotContainsAssertion(cfg domain.AssertionConfig) (ports.Assertion, error) {
	return &notContainsAssertion{field: cfg.Field, value: cfg.Value}, nil
}

func (a *notContainsAssertion) Assert(msg *domain.Message) error {
	field := strings.ToLower(a.field)
	actual := msg.Fields[field]
	if strings.Contains(actual, a.value) {
		return fmt.Errorf("%w: field %q: expected not to contain %q, got %q", domain.ErrValidation, a.field, a.value, actual)
	}
	return nil
}
