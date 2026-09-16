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
	"e2e-framework/internal/core/ports"
	"e2e-framework/internal/pkg/assertion"
	"e2e-framework/internal/pkg/httputil"
	"e2e-framework/internal/pkg/template"
)

type HTTPTrigger struct {
	client     ports.HttpClient
	assertions *assertion.Registry
}

func NewHTTPTrigger(registry *assertion.Registry, client ports.HttpClient) (*HTTPTrigger, error) {
	if registry == nil {
		return nil, fmt.Errorf("%w: assertion registry is required", domain.ErrConfiguration)
	}
	if client == nil {
		return nil, fmt.Errorf("%w: http client is required", domain.ErrConfiguration)
	}

	return &HTTPTrigger{
		client:     client,
		assertions: registry,
	}, nil
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

	var (
		reqBody  io.Reader
		bodyText string
	)
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
			bodyText = form.Encode()
			reqBody = strings.NewReader(bodyText)
		} else {
			b, err := json.Marshal(bodyMap)
			if err != nil {
				return nil, fmt.Errorf("%w: failed to serialize trigger body: %v", domain.ErrTriggerFailed, err)
			}
			bodyText = string(b)
			reqBody = bytes.NewReader(b)
		}
	}

	if reason := unresolvedTriggerReason(targetURL, headers, bodyText); reason != "" {
		return nil, fmt.Errorf("%w: %s", domain.ErrTriggerFailed, reason)
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

	// Unified assertions: same 13 types as receivers, same lowercased Fields + gjson on Raw
	for _, acfg := range def.ResponseAssertions {
		acfg.Field = template.ReplaceString(acfg.Field, vars)
		acfg.Value = template.ReplaceString(acfg.Value, vars)
		a, err := t.assertions.Create(acfg)
		if err != nil {
			return nil, fmt.Errorf("%w: assertion failed: %v | response body: %s", domain.ErrTriggerFailed, err, rawResp)
		}
		msg := &domain.Message{Fields: flatResp, Raw: rawResp}
		if err := a.Assert(msg); err != nil {
			return nil, fmt.Errorf("%w: assertion failed: %v | response body: %s", domain.ErrTriggerFailed, err, rawResp)
		}
	}

	extracted := make(map[string]string, len(def.Extract))
	for varName, jsonPath := range def.Extract {
		if val, ok := flatResp[strings.ToLower(jsonPath)]; ok {
			extracted[varName] = val
		}
	}

	return extracted, nil
}

func unresolvedTriggerReason(targetURL string, headers map[string]string, body string) string {
	if template.HasUnresolved(targetURL) {
		return fmt.Sprintf("trigger url %q contains an unresolved template placeholder", targetURL)
	}

	for k, v := range headers {
		if template.HasUnresolved(v) {
			return fmt.Sprintf("trigger header %q contains an unresolved template placeholder", k)
		}
	}

	if template.HasUnresolved(body) {
		return fmt.Sprintf("trigger body %q contains an unresolved template placeholder", body)
	}

	return ""
}
