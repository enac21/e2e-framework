package trigger

import (
	"fmt"
	"strings"

	"e2e-framework/internal/core/domain"
)

func NewPresentAssertion(cfg domain.AssertionConfig) ResponseAssertion {
	return func(ctx ResponseAssertionContext) error {
		field := strings.ToLower(ctx.Field)
		actual, exists := ctx.FlatResp[field]
		if !exists || actual == "" {
			return fmt.Errorf("field %q: expected to be present, but was empty or missing", ctx.Field)
		}

		return nil
	}
}
