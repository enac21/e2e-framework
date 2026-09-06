package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	triggerasserts "e2e-framework/internal/adapters/secondary/assertions/trigger"
	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/pkg/httputil"
	"e2e-framework/internal/pkg/template"
)

const defaultInterval = 5 * time.Second

// APIPollingReceiver polls an outbound HTTP endpoint with a configurable
// cadence. Each attempt validates the status code and the trigger-style
// response_assertions; the first attempt that passes both returns a
// domain.Message. Failures are transient — only the receiver timeout
// (managed by the orchestrator through the Collect context) fails the run.
type APIPollingReceiver struct {
	cfg        domain.ReceiverConfig
	assertions *triggerasserts.TriggerAssertionRegistry
	client     *http.Client
	runID      string
	vars       map[string]string
}

// NewAPIPollingReceiver builds the receiver and validates its configuration.
// A nil assertion registry falls back to the defaults; a nil client is replaced
// with a standard one (tests inject one pointing at a local test server).
func NewAPIPollingReceiver(cfg domain.ReceiverConfig, registry *triggerasserts.TriggerAssertionRegistry, client *http.Client) (*APIPollingReceiver, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("%w: api receiver requires 'url'", domain.ErrConfiguration)
	}

	if registry == nil {
		registry = triggerasserts.NewDefaultTriggerAssertionRegistry()
	}

	if client == nil {
		client = &http.Client{}
	}

	return &APIPollingReceiver{
		cfg:        cfg,
		assertions: registry,
		client:     client,
	}, nil
}

func (r *APIPollingReceiver) Start(ctx context.Context, runID string, vars map[string]string) error {
	r.runID = runID
	r.vars = vars

	return nil
}

func (r *APIPollingReceiver) Collect(ctx context.Context) (*domain.Message, error) {
	if r.runID == "" {
		return nil, fmt.Errorf("%w: receiver not started", domain.ErrConfiguration)
	}

	interval := r.cfg.Interval
	if interval <= 0 {
		interval = defaultInterval
	}

	var lastErr error

	for {
		msg, err := r.attempt(ctx)
		if err == nil {
			return msg, nil
		}

		lastErr = err

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: timeout waiting for API condition (last error: %v): %v", domain.ErrTimeout, lastErr, ctx.Err())
		case <-time.After(interval):
		}
	}
}

// attempt executes a single poll. It is considered successful when the status
// code matches expected_status (or is <400 when unset) and every
// response_assertion passes.
func (r *APIPollingReceiver) attempt(ctx context.Context) (*domain.Message, error) {
	vars := r.vars
	if vars == nil {
		vars = make(map[string]string)
	}
	vars["run_id"] = r.runID

	targetURL := template.ReplaceString(r.cfg.URL, vars)
	method := r.cfg.Method
	if method == "" {
		method = http.MethodGet
	}

	headers := template.ReplaceHeaders(r.cfg.Headers, vars)

	var (
		reqBody  io.Reader
		bodyText string
	)
	if r.cfg.Body != nil {
		bodyMap := template.ReplaceMap(r.cfg.Body, vars)

		isForm := false
		for k, v := range headers {
			if strings.ToLower(k) == "content-type" && strings.Contains(strings.ToLower(v), "application/x-www-form-urlencoded") {
				isForm = true
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
				return nil, fmt.Errorf("failed to serialize request body: %v", err)
			}
			bodyText = string(b)
			reqBody = bytes.NewReader(b)
		}
	}

	if reason := unresolvedReason(targetURL, headers, bodyText); reason != "" {
		return nil, fmt.Errorf("%w: %s", domain.ErrTriggerFailed, reason)
	}

	req, err := http.NewRequestWithContext(ctx, method, targetURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create polling request: %v", domain.ErrTriggerFailed, err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: HTTP request failed: %v", domain.ErrTriggerFailed, err)
	}
	defer resp.Body.Close()

	rawResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read response body: %v", domain.ErrTriggerFailed, err)
	}

	if r.cfg.ExpectedStatus != 0 {
		if resp.StatusCode != r.cfg.ExpectedStatus {
			return nil, fmt.Errorf("expected status %d, got %d: %s", r.cfg.ExpectedStatus, resp.StatusCode, string(rawResp))
		}
	} else if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("returned status %d: %s", resp.StatusCode, string(rawResp))
	}

	var flatResp map[string]string
	if len(bytes.TrimSpace(rawResp)) > 0 {
		var payload map[string]any
		if err := json.Unmarshal(rawResp, &payload); err == nil {
			flatResp = httputil.FlattenJSON(payload)
		}
	}

	if len(r.cfg.ResponseAsserts) > 0 {
		if len(bytes.TrimSpace(rawResp)) == 0 {
			return nil, fmt.Errorf("response_assertions configured but API response body is empty")
		}

		var respPayload map[string]any
		if err := json.Unmarshal(rawResp, &respPayload); err != nil {
			return nil, fmt.Errorf("failed to parse API response as JSON: %v", err)
		}

		flatResp = httputil.FlattenJSON(respPayload)

		if err := r.assertions.Run(r.cfg.ResponseAsserts, flatResp, rawResp, vars); err != nil {
			return nil, err
		}
	}

	respHeaders := make(map[string]string, len(resp.Header))
	for k, v := range resp.Header {
		if len(v) > 0 {
			respHeaders[k] = v[0]
		}
	}

	return &domain.Message{
		RunID:        r.runID,
		ReceiverType: domain.APIReceiverType,
		ReceivedAt:   time.Now(),
		Headers:      respHeaders,
		Fields:       flatResp,
		Raw:          rawResp,
	}, nil
}

func (r *APIPollingReceiver) Stop() error {
	return nil
}

func unresolvedReason(targetURL string, headers map[string]string, body string) string {
	if template.HasUnresolved(targetURL) {
		return fmt.Sprintf("api receiver url %q contains an unresolved template placeholder", targetURL)
	}

	for k, v := range headers {
		if template.HasUnresolved(v) {
			return fmt.Sprintf("api receiver header %q contains an unresolved template placeholder", k)
		}
	}

	if template.HasUnresolved(body) {
		return fmt.Sprintf("api receiver body %q contains an unresolved template placeholder", body)
	}

	return ""
}
