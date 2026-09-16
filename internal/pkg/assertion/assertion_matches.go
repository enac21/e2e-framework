package assertion

import (
	"fmt"
	"regexp"
	"strings"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type matchesAssertion struct {
	field   string
	pattern *regexp.Regexp
	raw     string
}

func NewMatchesAssertion(cfg domain.AssertionConfig) (ports.Assertion, error) {
	pattern, err := regexp.Compile(cfg.Value)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid regex pattern %q: %v", domain.ErrConfiguration, cfg.Value, err)
	}
	return &matchesAssertion{field: cfg.Field, pattern: pattern, raw: cfg.Value}, nil
}

func (a *matchesAssertion) Assert(msg *domain.Message) error {
	field := strings.ToLower(a.field)
	actual := msg.Fields[field]
	if !a.pattern.MatchString(actual) {
		return fmt.Errorf("%w: field %q: expected to match pattern %q, got %q", domain.ErrValidation, a.field, a.raw, actual)
	}
	return nil
}
