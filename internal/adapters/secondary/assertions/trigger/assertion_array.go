package trigger

import (
	"fmt"

	"github.com/tidwall/gjson"

	"e2e-framework/internal/core/domain"
)

func NewArrayContainsAssertion(cfg domain.AssertionConfig) ResponseAssertion {
	return pathContainsAssertion(cfg)
}

func NewMapContainsAssertion(cfg domain.AssertionConfig) ResponseAssertion {
	return pathContainsAssertion(cfg)
}

func pathContainsAssertion(cfg domain.AssertionConfig) ResponseAssertion {
	return func(ctx ResponseAssertionContext) error {
		if !walkFind(gjson.Get(string(ctx.RawBody), ctx.Field), ctx.Value) {
			return fmt.Errorf("field %q: no element with value %q found", ctx.Field, ctx.Value)
		}

		return nil
	}
}

func walkFind(r gjson.Result, target string) bool {
	if r.IsArray() {
		found := false
		r.ForEach(func(_, v gjson.Result) bool {
			if walkFind(v, target) {
				found = true
				return false
			}
			return true
		})
		return found
	}
	return r.String() == target
}
