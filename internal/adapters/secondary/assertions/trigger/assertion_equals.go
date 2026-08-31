package trigger

import (
	"fmt"
	"strings"

	"e2e-framework/internal/core/domain"
)

func NewEqualsAssertion(cfg domain.AssertionConfig) ResponseAssertion {
	return func(ctx ResponseAssertionContext) error {
		field := strings.ToLower(ctx.Field)
		actual := ctx.FlatResp[field]
		if actual != ctx.Value {
			return fmt.Errorf("field %q: expected %q, got %q", ctx.Field, ctx.Value, actual)
		}

		return nil
	}
}
