package assertion

import (
	"fmt"
	"regexp"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type MatchesAssertion struct {
	field   string
	pattern *regexp.Regexp
}

func NewMatchesAssertion(cfg domain.AssertionConfig) (ports.Assertion, error) {
	pattern, err := regexp.Compile(cfg.Value)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern %q: %w", cfg.Value, err)
	}
	return &MatchesAssertion{
		field:   cfg.Field,
		pattern: pattern,
	}, nil
}

func (a *MatchesAssertion) Assert(msg *domain.Message) error {
	actual := msg.Fields[a.field]
	if !a.pattern.MatchString(actual) {
		return fmt.Errorf("field %q: expected to match pattern %q, got %q", a.field, a.pattern.String(), actual)
	}
	return nil
}
