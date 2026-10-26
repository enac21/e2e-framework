package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/pkg/logger"
	"e2e-framework/internal/pkg/template"
)

const defaultCallTimeout = 15 * time.Second

type HTTPNotifier struct {
	client *http.Client
	logger *logger.Logger
}

func NewHTTPNotifier() *HTTPNotifier {
	return &HTTPNotifier{
		client: &http.Client{},
		logger: logger.New("notifier"),
	}
}

func (n *HTTPNotifier) Notify(ctx context.Context, cfg domain.OnFailureConfig, result *domain.TestResult) error {
	if result.Status == domain.StatusPassed || result.Status == domain.StatusSkipped {
		return nil
	}

	if len(cfg.Calls) == 0 {
		return nil
	}

	vars := map[string]string{
		"run_id":  result.RunID,
		"test_id": result.TestID,
		"error":   result.Error,
	}

	for k, v := range result.TriggerVars {
		vars[k] = v
	}

	var errs []error
	for i, call := range cfg.Calls {
		if err := n.executeCall(ctx, call, vars, result, i); err != nil {
			n.logger.Error().
				Str(logger.AttrRunID, result.RunID).
				Str(logger.AttrTestID, result.TestID).
				Int(logger.AttrCallIndex, i+1).
				Err(err).
				Msg("failure notification failed")

			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (n *HTTPNotifier) executeCall(ctx context.Context, call domain.CallAction, vars map[string]string, result *domain.TestResult, index int) error {
	if call.DelayBefore > 0 {
		time.Sleep(call.DelayBefore)
	}

	callStart := time.Now()

	method := call.Method
	if method == "" {
		method = http.MethodPost
	}

	resolvedURL := template.ReplaceString(call.URL, vars)
	resolvedHeaders := template.ReplaceHeaders(call.Headers, vars)

	var bodyBytes []byte
	if call.Body != nil {
		bodyMap := template.ReplaceMap(call.Body, vars)

		b, err := json.Marshal(bodyMap)
		if err != nil {
			return fmt.Errorf("%w: failed to serialize on_failure body: %v", domain.ErrInternal, err)
		}

		bodyBytes = b
	}

	if reason := unresolvedReason(resolvedURL, resolvedHeaders, bodyBytes); reason != "" {
		n.logger.Warn().
			Str(logger.AttrRunID, result.RunID).
			Str(logger.AttrTestID, result.TestID).
			Int(logger.AttrCallIndex, index+1).
			Str(logger.AttrReason, reason).
			Msg("failure notification skipped")

		return nil
	}

	var reqBody io.Reader
	if bodyBytes != nil {
		reqBody = bytes.NewReader(bodyBytes)
	}

	timeout := call.Timeout
	if timeout == 0 {
		timeout = defaultCallTimeout
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, resolvedURL, reqBody)
	if err != nil {
		return fmt.Errorf("%w: failed to create on_failure request: %v", domain.ErrInternal, err)
	}

	for k, v := range resolvedHeaders {
		req.Header.Set(k, v)
	}

	if req.Header.Get("Content-Type") == "" && reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: on_failure HTTP request failed: %v", domain.ErrInternal, err)
	}
	defer resp.Body.Close()

	if call.ExpectedStatus != 0 && resp.StatusCode != call.ExpectedStatus {
		return fmt.Errorf("%w: on_failure call returned status %d, expected %d", domain.ErrInternal, resp.StatusCode, call.ExpectedStatus)
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)

		return fmt.Errorf("%w: on_failure call returned status %d: %s", domain.ErrInternal, resp.StatusCode, string(body))
	}

	n.logger.Info().
		Str(logger.AttrRunID, result.RunID).
		Str(logger.AttrTestID, result.TestID).
		Int(logger.AttrCallIndex, index+1).
		Int(logger.AttrRespStatus, resp.StatusCode).
		Duration(logger.AttrDurationMS, time.Since(callStart)).
		Msg("failure notification sent")

	return nil
}

// unresolvedReason returns a human-readable description of the first
// template placeholder that could not be resolved, or an empty string
// when every value is fully resolved.
func unresolvedReason(url string, headers map[string]string, body []byte) string {
	if template.HasUnresolved(url) {
		return fmt.Sprintf("unresolved variable in url %q", url)
	}

	for k, v := range headers {
		if template.HasUnresolved(v) {
			return fmt.Sprintf("unresolved variable in header %q (%q)", k, v)
		}
	}

	if template.HasUnresolved(string(body)) {
		return fmt.Sprintf("unresolved variable in body %q", string(body))
	}

	return ""
}
