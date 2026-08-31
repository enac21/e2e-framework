package trigger

import (
	"fmt"
	"regexp"
	"strings"

	"e2e-framework/internal/core/domain"
)

func NewMatchesAssertion(cfg domain.AssertionConfig) ResponseAssertion {
	return func(ctx ResponseAssertionContext) error {
		re, err := regexp.Compile(ctx.Value)
		if err != nil {
			return fmt.Errorf("field %q: invalid regex pattern %q: %v", ctx.Field, ctx.Value, err)
		}

		field := strings.ToLower(ctx.Field)
		actual := ctx.FlatResp[field]
		if !re.MatchString(actual) {
			return fmt.Errorf("field %q: expected to match pattern %q, got %q", ctx.Field, ctx.Value, actual)
		}

		return nil
	}
}
