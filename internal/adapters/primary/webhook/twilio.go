package webhook

import (
	"log"
	"net/http"
	"strings"
	"time"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/pkg/httputil"
)

type TwilioExtractor struct{}

func NewTwilioExtractor() *TwilioExtractor {
	return &TwilioExtractor{}
}

func (e *TwilioExtractor) Extract(req *http.Request) (*domain.Message, error) {
	fields, raw, err := httputil.ExtractFields(req)
	if err != nil {
		return nil, err
	}

	log.Printf("[Twilio Webhook] Extracted fields: %v", fields)

	runID := strings.TrimSpace(fields["body"])

	return &domain.Message{
		RunID:        runID,
		ReceiverType: "sms",
		ReceivedAt:   time.Now(),
		Headers: map[string]string{
			"content-type": req.Header.Get("Content-Type"),
		},
		Fields: fields,
		Raw:    raw,
	}, nil
}
