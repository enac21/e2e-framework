package trigger

import (
	"fmt"
	"strings"

	"e2e-framework/internal/core/domain"
)

func NewLengthAssertion(cfg domain.AssertionConfig) ResponseAssertion {
	return func(ctx ResponseAssertionContext) error {
		field := strings.ToLower(ctx.Field)
		lenKey := field + ".__len__"
		actualLen, lenExists := ctx.FlatResp[lenKey]
		if !lenExists {
			return fmt.Errorf("field %q: field is not an array or does not exist", ctx.Field)
		}

		if actualLen != ctx.Value {
			return fmt.Errorf("field %q: expected length %s, got %s", ctx.Field, ctx.Value, actualLen)
		}

		return nil
	}
}
