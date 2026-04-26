package webhook

import (
	"log"
	"net/http"
	"time"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/pkg/httputil"
)

type MetaExtractor struct{}

func NewMetaExtractor() *MetaExtractor {
	return &MetaExtractor{}
}

func (e *MetaExtractor) Extract(req *http.Request) (*domain.Message, error) {
	fields, raw, err := httputil.ExtractFields(req)
	if err != nil {
		return nil, err
	}

	log.Printf("[Meta Webhook] Extracted fields: %v", fields)

	runID := fields["messages.0.text.body"]
	if runID == "" {
		runID = "unknown"
	}

	return &domain.Message{
		RunID:        runID,
		ReceiverType: "push",
		ReceivedAt:   time.Now(),
		Headers: map[string]string{
			"content-type": req.Header.Get("Content-Type"),
		},
		Fields: fields,
		Raw:    raw,
	}, nil
}
