package trigger

import (
	"fmt"
	"strconv"
	"strings"

	"e2e-framework/internal/core/domain"
)

func NewIntEqAssertion(cfg domain.AssertionConfig) ResponseAssertion {
	return intCompareAssertion(cfg, "int_eq")
}

func NewIntGtAssertion(cfg domain.AssertionConfig) ResponseAssertion {
	return intCompareAssertion(cfg, "int_gt")
}

func NewIntGteAssertion(cfg domain.AssertionConfig) ResponseAssertion {
	return intCompareAssertion(cfg, "int_gte")
}

func NewIntLtAssertion(cfg domain.AssertionConfig) ResponseAssertion {
	return intCompareAssertion(cfg, "int_lt")
}

func NewIntLteAssertion(cfg domain.AssertionConfig) ResponseAssertion {
	return intCompareAssertion(cfg, "int_lte")
}

func intCompareAssertion(cfg domain.AssertionConfig, typ string) ResponseAssertion {
	return func(ctx ResponseAssertionContext) error {
		field := strings.ToLower(ctx.Field)
		actualNum, actualInt := parseInt(ctx.FlatResp[field])
		valueNum, valueInt := parseInt(ctx.Value)
		if !actualInt {
			return fmt.Errorf("field %q: %s requires an integer field value, got %q", ctx.Field, typ, ctx.FlatResp[field])
		}

		if !valueInt {
			return fmt.Errorf("field %q: %s requires an integer expected value, got %q", ctx.Field, typ, ctx.Value)
		}

		if passes, symbol := compareInts(typ, actualNum, valueNum); !passes {
			return fmt.Errorf("field %q: %s failed: got %d, want %s %d", ctx.Field, typ, actualNum, symbol, valueNum)
		}

		return nil
	}
}

func parseInt(s string) (int64, bool) {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, false
	}

	return n, true
}

func compareInts(typ string, a, b int64) (passes bool, symbol string) {
	switch typ {
	case "int_eq":
		return a == b, "="
	case "int_gt":
		return a > b, ">"
	case "int_gte":
		return a >= b, ">="
	case "int_lt":
		return a < b, "<"
	case "int_lte":
		return a <= b, "<="
	}

	return false, typ
}
