package providers

import (
	"errors"
	"maps"
	"net/http"
	"strings"
	"time"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/pkg/httputil"
)

type GenericExtractor struct{}

func NewGenericExtractor() *GenericExtractor {
	return &GenericExtractor{}
}

func (e *GenericExtractor) Extract(req *http.Request) (*domain.Message, error) {
	var (
		fields = make(map[string]string)
		raw    = make([]byte, 0)
		err    error
	)

	if req.Body != nil {
		fields, raw, err = httputil.ExtractFields(req)
		if err != nil {
			if errors.Is(err, domain.ErrValidation) {
				if raw == nil || len(strings.TrimSpace(string(raw))) == 0 {
					fields = make(map[string]string)
					raw = []byte{}
				} else {
					return nil, err
				}
			} else {
				return nil, err
			}
		}
		if fields == nil {
			fields = make(map[string]string)
		}
		if raw == nil {
			raw = []byte{}
		}
	}

	for k, v := range maps.Clone(fields) {
		fields["body."+k] = v
	}

	for k, values := range req.Header {
		fields["headers."+strings.ToLower(k)] = strings.Join(values, ",")
	}

	for k, values := range req.URL.Query() {
		fields["query."+strings.ToLower(k)] = strings.Join(values, ",")
	}

	fields["method"] = req.Method

	runID := req.URL.Query().Get("run_id")
	if runID == "" {
		runID = req.Header.Get("X-E2E-Run-ID")
	}
	if runID == "" {
		if v, ok := fields["run_id"]; ok && strings.TrimSpace(v) != "" {
			runID = strings.TrimSpace(v)
		} else if v, ok := fields["runid"]; ok && strings.TrimSpace(v) != "" {
			runID = strings.TrimSpace(v)
		}
	}

	return &domain.Message{
		RunID:        runID,
		ReceiverType: domain.WebhookReceiverType,
		ReceivedAt:   time.Now(),
		Headers: map[string]string{
			"content-type": req.Header.Get("Content-Type"),
		},
		Fields: fields,
		Raw:    raw,
	}, nil
}
