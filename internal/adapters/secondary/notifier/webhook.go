package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/pkg/template"
)

type WebhookNotifier struct {
	client *http.Client
}

func NewWebhookNotifier() *WebhookNotifier {
	return &WebhookNotifier{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (n *WebhookNotifier) Notify(ctx context.Context, cfg domain.OnFailureConfig, result *domain.TestResult) error {
	// Only notify on failures or errors
	if result.Status == domain.StatusPassed || result.Status == domain.StatusSkipped {
		return nil
	}

	action := cfg.Webhook
	if action.URL == "" {
		return nil // No webhook configured
	}

	vars := map[string]string{
		"run_id":  result.RunID,
		"test_id": result.TestID,
		"error":   result.Error,
	}

	url := template.ReplaceString(action.URL, vars)
	method := action.Method
	if method == "" {
		method = http.MethodPost
	}

	headers := template.ReplaceHeaders(action.Headers, vars)

	var reqBody io.Reader
	if action.Body != nil {
		bodyMap := template.ReplaceMap(action.Body, vars)
		b, err := json.Marshal(bodyMap)
		if err != nil {
			return fmt.Errorf("%w: failed to serialize notifier body: %v", domain.ErrInternal, err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return fmt.Errorf("%w: failed to create notifier request: %v", domain.ErrInternal, err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	if req.Header.Get("Content-Type") == "" && reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: notifier HTTP request failed: %v", domain.ErrInternal, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)

		return fmt.Errorf("%w: notifier returned status %d: %s", domain.ErrInternal, resp.StatusCode, string(respBody))
	}

	return nil
}
