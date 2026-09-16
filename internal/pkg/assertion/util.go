package assertion

import (
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

// ParseInt parses s as int64 after trimming spaces.
func ParseInt(s string) (int64, bool) {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, false
	}

	return n, true
}

// CompareInts compares a and b according to typ.
func CompareInts(typ string, a, b int64) (bool, string) {
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

// WalkFind recursively searches gjson result for target string.
// Supports nested arrays: data.#.statuses.#.general_status
func WalkFind(r gjson.Result, target string) bool {
	if r.IsArray() {
		found := false
		r.ForEach(func(_, v gjson.Result) bool {
			if WalkFind(v, target) {
				found = true
				return false
			}
			return true
		})
		return found
	}
	return r.String() == target
}
