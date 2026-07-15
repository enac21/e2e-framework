package template

import (
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"
)

type generatorFunc func(args string) (string, bool)

// generators maps function name to its generator implementation.
// Add new entries here to support additional {{funcName(args)}} tags.
var generators = map[string]generatorFunc{
	"randomInt": generateRandomInt,
}

var generatorRegex = regexp.MustCompile(`\{\{(\w+)\(([^)]*)\)\}\}`)

func resolveGenerators(s string) string {
	return generatorRegex.ReplaceAllStringFunc(s, func(match string) string {
		sub := generatorRegex.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}

		fn, ok := generators[sub[1]]
		if !ok {
			return match
		}

		result, generated := fn(sub[2])
		if !generated {
			return match
		}

		return result
	})
}

func generateRandomInt(args string) (string, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(args))
	if err != nil || n <= 0 {
		return "", false
	}

	max := 1
	for range n {
		max *= 10
	}

	return strconv.Itoa(rand.IntN(max)), true
}

func ReplaceString(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{{"+k+"}}", v)
	}

	return resolveGenerators(s)
}

func ReplaceHeaders(headers map[string]string, vars map[string]string) map[string]string {
	if headers == nil {
		return nil
	}

	res := make(map[string]string, len(headers))
	for k, v := range headers {
		res[k] = ReplaceString(v, vars)
	}

	return res
}

func ReplaceMap(m map[string]any, vars map[string]string) map[string]any {
	if m == nil {
		return nil
	}

	res := make(map[string]any, len(m))
	for k, v := range m {
		res[k] = replaceAny(v, vars)
	}

	return res
}

func replaceAny(v any, vars map[string]string) any {
	switch val := v.(type) {
	case string:
		return ReplaceString(val, vars)
	case map[string]any:
		return ReplaceMap(val, vars)
	case map[any]any:
		res := make(map[string]any, len(val))
		for mk, mv := range val {
			if sk, ok := mk.(string); ok {
				res[sk] = replaceAny(mv, vars)
			}
		}

		return res
	case []any:
		res := make([]any, len(val))
		for i, item := range val {
			res[i] = replaceAny(item, vars)
		}

		return res
	default:
		return val
	}
}
