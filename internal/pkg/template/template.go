package template

import (
	"regexp"
	"strings"

	"e2e-framework/internal/pkg/template/generators"
)

var (
	generatorRegex   = regexp.MustCompile(`\{\{(\w+)\(([^)]*)\)\}\}`)
	placeholderRegex = regexp.MustCompile(`\{\{[^{}]*\}\}`)
)

func resolveGenerators(s string) string {
	return generatorRegex.ReplaceAllStringFunc(s, func(match string) string {
		sub := generatorRegex.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}

		result, generated := generators.Resolve(sub[1], sub[2])
		if !generated {
			return match
		}

		return result
	})
}

func ReplaceString(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{{"+k+"}}", v)
	}

	return resolveGenerators(s)
}

func HasUnresolved(s string) bool {
	return placeholderRegex.MatchString(s)
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
