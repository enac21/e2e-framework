package trigger

import (
	"fmt"
	"strings"

	"e2e-framework/internal/core/domain"
)

func NewContainsAssertion(cfg domain.AssertionConfig) ResponseAssertion {
	return func(ctx ResponseAssertionContext) error {
		field := strings.ToLower(ctx.Field)
		actual := ctx.FlatResp[field]
		if !strings.Contains(actual, ctx.Value) {
			return fmt.Errorf("field %q: expected to contain %q, got %q", ctx.Field, ctx.Value, actual)
		}

		return nil
	}
}

func NewNotContainsAssertion(cfg domain.AssertionConfig) ResponseAssertion {
	return func(ctx ResponseAssertionContext) error {
		field := strings.ToLower(ctx.Field)
		actual := ctx.FlatResp[field]
		if strings.Contains(actual, ctx.Value) {
			return fmt.Errorf("field %q: expected not to contain %q, got %q", ctx.Field, ctx.Value, actual)
		}

		return nil
	}
}
