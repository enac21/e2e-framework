package assertion

import (
	"fmt"
	"strings"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type ContainsAssertion struct {
	field string
	value string
}

func NewContainsAssertion(cfg domain.AssertionConfig) (ports.Assertion, error) {
	return &ContainsAssertion{
		field: cfg.Field,
		value: cfg.Value,
	}, nil
}

func (a *ContainsAssertion) Assert(msg *domain.Message) error {
	actual := msg.Fields[a.field]
	if !strings.Contains(actual, a.value) {
		return fmt.Errorf("field %q: expected to contain %q, got %q", a.field, a.value, actual)
	}
	return nil
}
