package webhook

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"e2e-framework/internal/core/domain"
)

type TwilioExtractor struct{}

func NewTwilioExtractor() Extractor {
	return &TwilioExtractor{}
}

func (e *TwilioExtractor) Extract(req *http.Request) (*domain.Message, error) {
	if err := req.ParseForm(); err != nil {
		return nil, fmt.Errorf("failed to parse twilio form: %w", err)
	}

	from := req.FormValue("From")
	to := req.FormValue("To")
	body := req.FormValue("Body")
	runID := req.FormValue("MessageSid") // In a real scenario, map this or extract from body

	// For e2e framework MVP, we assume runID is sent in the body or specific field
	// Let's assume the body contains the runID for correlation.
	if len(body) >= 19 && body[:3] == "ID:" {
		runID = body[3:19] // Simplified extraction example
	}

	raw, _ := json.Marshal(req.Form)

	return &domain.Message{
		RunID:        runID,
		ReceiverType: "sms",
		ReceivedAt:   time.Now(),
		Headers: map[string]string{
			"content-type": req.Header.Get("Content-Type"),
		},
		Fields: map[string]string{
			"from": from,
			"to":   to,
			"body": body,
		},
		Raw: raw,
	}, nil
}
