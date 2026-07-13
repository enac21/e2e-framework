package httputil

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"

	"e2e-framework/internal/core/domain"
)

func ExtractFields(req *http.Request) (map[string]string, []byte, error) {
	contentType := strings.ToLower(req.Header.Get("Content-Type"))

	if strings.Contains(contentType, "application/json") {
		return extractJSON(req)
	}

	return extractForm(req)
}

func extractJSON(req *http.Request) (map[string]string, []byte, error) {
	raw, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: failed to read request body: %v", domain.ErrInternal, err)
	}

	if req.Body != nil {
		defer req.Body.Close()
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, fmt.Errorf("%w: failed to parse JSON payload: %v", domain.ErrValidation, err)
	}

	fields := flattenMap("", payload)

	return fields, raw, nil
}

func extractForm(req *http.Request) (map[string]string, []byte, error) {
	if err := req.ParseForm(); err != nil {
		return nil, nil, fmt.Errorf("%w: failed to parse form payload: %v", domain.ErrValidation, err)
	}

	fields := make(map[string]string, len(req.Form))
	for k, values := range req.Form {
		fields[strings.ToLower(k)] = strings.Join(values, ",")
	}

	raw, _ := json.Marshal(fields)

	return fields, raw, nil
}

func FlattenJSON(m map[string]any) map[string]string {
	return flattenMap("", m)
}

func flattenMap(prefix string, m map[string]any) map[string]string {
	result := make(map[string]string)
	for k, v := range m {
		key := strings.ToLower(k)
		if prefix != "" {
			key = prefix + "." + key
		}

		switch val := v.(type) {
		case map[string]any:
			for nested, nv := range flattenMap(key, val) {
				result[nested] = nv
			}
		case []any:
			result[key+".__len__"] = fmt.Sprintf("%d", len(val))
			for i, elem := range val {
				indexKey := fmt.Sprintf("%s.%d", key, i)
				switch ev := elem.(type) {
				case map[string]any:
					maps.Copy(result, flattenMap(indexKey, ev))
				default:
					result[indexKey] = fmt.Sprintf("%v", ev)
				}
			}
		default:
			result[key] = fmt.Sprintf("%v", val)
		}
	}

	return result
}
