package assertion

import (
	"fmt"

	"github.com/tidwall/gjson"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type arrayContainsAssertion struct {
	field string
	value string
}

func NewArrayContainsAssertion(cfg domain.AssertionConfig) (ports.Assertion, error) {
	return &arrayContainsAssertion{field: cfg.Field, value: cfg.Value}, nil
}

func (a *arrayContainsAssertion) Assert(msg *domain.Message) error {
	if !WalkFind(gjson.Get(string(msg.Raw), a.field), a.value) {
		return fmt.Errorf("%w: field %q: no element with value %q found", domain.ErrValidation, a.field, a.value)
	}
	return nil
}

type mapContainsAssertion struct {
	field string
	value string
}

func NewMapContainsAssertion(cfg domain.AssertionConfig) (ports.Assertion, error) {
	return &mapContainsAssertion{field: cfg.Field, value: cfg.Value}, nil
}

func (a *mapContainsAssertion) Assert(msg *domain.Message) error {
	if !WalkFind(gjson.Get(string(msg.Raw), a.field), a.value) {
		return fmt.Errorf("%w: field %q: no element with value %q found", domain.ErrValidation, a.field, a.value)
	}
	return nil
}
