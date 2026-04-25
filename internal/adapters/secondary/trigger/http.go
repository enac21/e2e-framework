package trigger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"e2e-framework/internal/core/domain"
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

func (t *HTTPTrigger) Execute(ctx context.Context, def domain.TriggerConfig, runID string) error {
	vars := map[string]string{
		"run_id": runID,
	}

	url := template.ReplaceString(def.URL, vars)
	method := def.Method
	if method == "" {
		method = http.MethodGet
	}

	headers := template.ReplaceHeaders(def.Headers, vars)

	var reqBody io.Reader
	if def.Body != nil {
		bodyMap := template.ReplaceMap(def.Body, vars)
		b, err := json.Marshal(bodyMap)
		if err != nil {
			return fmt.Errorf("failed to serialize trigger body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	reqCtx := ctx
	if def.Timeout > 0 {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, def.Timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(reqCtx, method, url, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create trigger request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("trigger HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("trigger returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
