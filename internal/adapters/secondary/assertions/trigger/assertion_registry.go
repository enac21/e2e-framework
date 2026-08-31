package trigger

import (
	"fmt"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/pkg/template"
)

// ResponseAssertionContext carries everything a trigger response assertion
// needs to evaluate against the HTTP response of a trigger step.
type ResponseAssertionContext struct {
	Field    string
	Value    string
	FlatResp map[string]string
	RawBody  []byte
}

type ResponseAssertion func(ctx ResponseAssertionContext) error

type ResponseAssertionFactory func(cfg domain.AssertionConfig) ResponseAssertion

type TriggerAssertionRegistry struct {
	factories map[string]ResponseAssertionFactory
}

func NewTriggerAssertionRegistry() *TriggerAssertionRegistry {
	return &TriggerAssertionRegistry{
		factories: make(map[string]ResponseAssertionFactory),
	}
}

func (r *TriggerAssertionRegistry) Register(typeName string, factory ResponseAssertionFactory) {
	r.factories[typeName] = factory
}

func (r *TriggerAssertionRegistry) Create(cfg domain.AssertionConfig) (ResponseAssertion, error) {
	factory, ok := r.factories[cfg.Type]
	if !ok {
		return nil, fmt.Errorf("%w: unknown response_assertions type: %q", domain.ErrConfiguration, cfg.Type)
	}

	return factory(cfg), nil
}

func NewDefaultTriggerAssertionRegistry() *TriggerAssertionRegistry {
	r := NewTriggerAssertionRegistry()
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

// Run evaluates every configured assertion against the trigger response,
// resolving template variables in the expected value first.
func (r *TriggerAssertionRegistry) Run(assertions []domain.AssertionConfig, flatResp map[string]string, rawBody []byte, vars map[string]string) error {
	for _, cfg := range assertions {
		assertion, err := r.Create(cfg)
		if err != nil {
			return fmt.Errorf("%w: response assertion failed: %v | response body: %s", domain.ErrTriggerFailed, err, rawBody)
		}

		ctx := ResponseAssertionContext{
			Field:    cfg.Field,
			Value:    template.ReplaceString(cfg.Value, vars),
			FlatResp: flatResp,
			RawBody:  rawBody,
		}

		if err := assertion(ctx); err != nil {
			return fmt.Errorf("%w: response assertion failed: %v | response body: %s", domain.ErrTriggerFailed, err, rawBody)
		}
	}

	return nil
}
