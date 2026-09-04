package template

import (
	"regexp"
	"strconv"
	"strings"

	"e2e-framework/internal/pkg/template/generators"
)

var (
	generatorRegex   = regexp.MustCompile(`\{\{(\w+)\(([^)]*)\)\}\}`)
	incrementRegex   = regexp.MustCompile(`\{\{\+\+\(([^)]*)\)\}\}`)
	decrementRegex   = regexp.MustCompile(`\{\{--\(([^)]*)\)\}\}`)
	placeholderRegex = regexp.MustCompile(`\{\{[^{}]*\}\}`)
)

func ReplaceString(s string, vars map[string]string) string {
	s = resolveIncrements(s, vars)

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

func resolveIncrements(s string, vars map[string]string) string {
	s = replaceIncrements(s, vars, incrementRegex, 1)

	return replaceIncrements(s, vars, decrementRegex, -1)
}

func replaceIncrements(s string, vars map[string]string, re *regexp.Regexp, delta int64) string {
	return re.ReplaceAllStringFunc(s, func(match string) string {
		sub := re.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}

		name := strings.TrimSpace(sub[1])
		raw, exists := vars[name]
		cur, ok := parseVarInt(raw)
		if !ok {
			if exists {
				return match
			}
			cur = 0
		}

		vars[name] = strconv.FormatInt(cur+delta, 10)

		return vars[name]
	})
}

func parseVarInt(s string) (int64, bool) {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, false
	}

	return n, true
}
