package assertion

import (
	"fmt"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
	"e2e-framework/internal/pkg/template"
)

type Factory func(cfg domain.AssertionConfig) (ports.Assertion, error)

type Registry struct {
	factories map[string]Factory
}

func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]Factory)}
}

func (r *Registry) Register(typeName string, factory Factory) {
	r.factories[typeName] = factory
}

func (r *Registry) Create(cfg domain.AssertionConfig) (ports.Assertion, error) {
	if factory, ok := r.factories[cfg.Type]; ok {
		return factory(cfg)
	}

	return nil, fmt.Errorf("%w: unknown assertion type: %q", domain.ErrConfiguration, cfg.Type)
}

// NewDefaultRegistry returns a registry with all 13 assertion types.
func NewDefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register("equals", NewEqualsAssertion)
	r.Register("contains", NewContainsAssertion)
	r.Register("not_contains", NewNotContainsAssertion)
	r.Register("present", NewPresentAssertion)
	r.Register("matches", NewMatchesAssertion)
	r.Register("array_contains", NewArrayContainsAssertion)
	r.Register("map_contains", NewMapContainsAssertion)
	r.Register("length", NewLengthAssertion)
	r.Register("int_eq", NewIntEqAssertion)
	r.Register("int_gt", NewIntGtAssertion)
	r.Register("int_gte", NewIntGteAssertion)
	r.Register("int_lt", NewIntLtAssertion)
	r.Register("int_lte", NewIntLteAssertion)

	return r
}

// Run evaluates every assertion against a synthetic message built from flatResp/Raw.
// It handles {{variable}} substitution on Field and Value before creation.
func (r *Registry) Run(assertions []domain.AssertionConfig, flatResp map[string]string, rawBody []byte, vars map[string]string) error {
	for _, cfg := range assertions {
		cfg.Field = template.ReplaceString(cfg.Field, vars)
		cfg.Value = template.ReplaceString(cfg.Value, vars)
		a, err := r.Create(cfg)
		if err != nil {
			return fmt.Errorf("%w: assertion failed: %v | response body: %s", domain.ErrTriggerFailed, err, rawBody)
		}
		msg := &domain.Message{
			Fields: flatResp,
			Raw:    rawBody,
		}
		if err := a.Assert(msg); err != nil {
			return fmt.Errorf("%w: assertion failed: %v | response body: %s", domain.ErrTriggerFailed, err, rawBody)
		}
	}

	return nil
}
