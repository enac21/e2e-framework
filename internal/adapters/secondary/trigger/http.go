package trigger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/pkg/httputil"
	"e2e-framework/internal/pkg/template"
)

type HTTPTrigger struct {
	client *http.Client
}

func NewHTTPTrigger() *HTTPTrigger {
	return &HTTPTrigger{
		client: &http.Client{},
	}
}

func (t *HTTPTrigger) Execute(ctx context.Context, def domain.TriggerConfig, runID string, vars map[string]string) (map[string]string, error) {
	if vars == nil {
		vars = make(map[string]string)
	}
	vars["run_id"] = runID

	targetURL := template.ReplaceString(def.URL, vars)
	method := def.Method
	if method == "" {
		method = http.MethodGet
	}

	headers := template.ReplaceHeaders(def.Headers, vars)

	var reqBody io.Reader
	if def.Body != nil {
		bodyMap := template.ReplaceMap(def.Body, vars)

		isForm := false
		for k, v := range headers {
			if strings.ToLower(k) == "content-type" {
				if strings.Contains(strings.ToLower(v), "application/x-www-form-urlencoded") {
					isForm = true
				}
			}
		}

		if isForm {
			form := url.Values{}
			for k, v := range bodyMap {
				form.Set(k, fmt.Sprintf("%v", v))
			}
			reqBody = strings.NewReader(form.Encode())
		} else {
			b, err := json.Marshal(bodyMap)
			if err != nil {
				return nil, fmt.Errorf("%w: failed to serialize trigger body: %v", domain.ErrTriggerFailed, err)
			}
			reqBody = bytes.NewReader(b)
		}
	}

	reqCtx := ctx
	if def.Timeout > 0 {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, def.Timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(reqCtx, method, targetURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create trigger request: %v", domain.ErrTriggerFailed, err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: HTTP request failed: %v", domain.ErrTriggerFailed, err)
	}
	defer resp.Body.Close()

	if def.ExpectedStatus != 0 {
		if resp.StatusCode != def.ExpectedStatus {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("%w: expected status %d, got %d: %s", domain.ErrTriggerFailed, def.ExpectedStatus, resp.StatusCode, string(body))
		}
	} else if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: returned status %d: %s", domain.ErrTriggerFailed, resp.StatusCode, string(body))
	}

	if len(def.Extract) == 0 && len(def.ResponseAssertions) == 0 {
		return map[string]string{}, nil
	}

	rawResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read trigger response body: %v", domain.ErrTriggerFailed, err)
	}

	if len(bytes.TrimSpace(rawResp)) == 0 {
		return nil, fmt.Errorf("%w: extract/response_assertions configured but trigger response body is empty", domain.ErrTriggerFailed)
	}

	var respPayload map[string]any
	if err := json.Unmarshal(rawResp, &respPayload); err != nil {
		return nil, fmt.Errorf("%w: failed to parse trigger response as JSON: %v", domain.ErrTriggerFailed, err)
	}

	flatResp := httputil.FlattenJSON(respPayload)

	if err := runResponseAssertions(def.ResponseAssertions, flatResp, rawResp, vars); err != nil {
		return nil, err
	}

	extracted := make(map[string]string, len(def.Extract))
	for varName, jsonPath := range def.Extract {
		if val, ok := flatResp[strings.ToLower(jsonPath)]; ok {
			extracted[varName] = val
		}
	}

	return extracted, nil
}

func runResponseAssertions(assertions []domain.AssertionConfig, flatResp map[string]string, rawBody []byte, vars map[string]string) error {
	for _, cfg := range assertions {
		var (
			resolvedValue  = template.ReplaceString(cfg.Value, vars)
			field          = strings.ToLower(cfg.Field)
			actual, exists = flatResp[field]
			assertErr      error
		)

		switch cfg.Type {
		case "equals":
			if actual != resolvedValue {
				assertErr = fmt.Errorf("field %q: expected %q, got %q", cfg.Field, resolvedValue, actual)
			}
		case "contains":
			if !strings.Contains(actual, resolvedValue) {
				assertErr = fmt.Errorf("field %q: expected to contain %q, got %q", cfg.Field, resolvedValue, actual)
			}
		case "not_contains":
			if strings.Contains(actual, resolvedValue) {
				assertErr = fmt.Errorf("field %q: expected not to contain %q, got %q", cfg.Field, resolvedValue, actual)
			}
		case "present":
			if !exists || actual == "" {
				assertErr = fmt.Errorf("field %q: expected to be present, but was empty or missing", cfg.Field)
			}
		case "matches":
			re, compErr := regexp.Compile(resolvedValue)
			if compErr != nil {
				assertErr = fmt.Errorf("field %q: invalid regex pattern %q: %v", cfg.Field, resolvedValue, compErr)
			} else if !re.MatchString(actual) {
				assertErr = fmt.Errorf("field %q: expected to match pattern %q, got %q", cfg.Field, resolvedValue, actual)
			}
		case "array_contains", "map_contains":
			if !walkFind(gjson.Get(string(rawBody), cfg.Field), resolvedValue) {
				assertErr = fmt.Errorf("field %q: no element with value %q found", cfg.Field, resolvedValue)
			}
		case "length":
			lenKey := field + ".__len__"
			actualLen, lenExists := flatResp[lenKey]
			if !lenExists {
				assertErr = fmt.Errorf("field %q: field is not an array or does not exist", cfg.Field)
			} else if actualLen != resolvedValue {
				assertErr = fmt.Errorf("field %q: expected length %s, got %s", cfg.Field, resolvedValue, actualLen)
			}
		default:
			assertErr = fmt.Errorf("unknown response_assertions type %q", cfg.Type)
		}

		if assertErr != nil {
			return fmt.Errorf("%w: response assertion failed: %v | response body: %s", domain.ErrTriggerFailed, assertErr, rawBody)
		}
	}

	return nil
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
