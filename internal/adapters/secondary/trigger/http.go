package trigger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

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

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)

		return nil, fmt.Errorf("%w: returned status %d: %s", domain.ErrTriggerFailed, resp.StatusCode, string(respBody))
	}

	if len(def.Extract) == 0 {
		return map[string]string{}, nil
	}

	rawResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read trigger response body: %v", domain.ErrTriggerFailed, err)
	}

	if len(bytes.TrimSpace(rawResp)) == 0 {
		return nil, fmt.Errorf("%w: extraction was configured but the trigger response body is empty", domain.ErrTriggerFailed)
	}

	var respPayload map[string]any
	if err := json.Unmarshal(rawResp, &respPayload); err != nil {
		return nil, fmt.Errorf("%w: failed to parse trigger response as JSON: %v", domain.ErrTriggerFailed, err)
	}

	flatResp := httputil.FlattenJSON(respPayload)

	extracted := make(map[string]string, len(def.Extract))
	for varName, jsonPath := range def.Extract {
		if val, ok := flatResp[strings.ToLower(jsonPath)]; ok {
			extracted[varName] = val
		}
	}

	return extracted, nil
}
