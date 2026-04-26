package webhook

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"e2e-framework/internal/core/domain"
)

type TwilioExtractor struct{}

func NewTwilioExtractor() Extractor {
	return &TwilioExtractor{}
}

func (e *TwilioExtractor) Extract(req *http.Request) (*domain.Message, error) {
	contentType := req.Header.Get("Content-Type")

	var from, to, body string
	var raw []byte

	if strings.Contains(strings.ToLower(contentType), "application/json") {
		var payload map[string]any
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read json body: %w", err)
		}
		defer req.Body.Close()
		raw = b

		if err := json.Unmarshal(b, &payload); err == nil {
			if f, ok := payload["From"].(string); ok {
				from = f
			} else if f, ok := payload["from"].(string); ok {
				from = f
			}
			
			if t, ok := payload["To"].(string); ok {
				to = t
			} else if t, ok := payload["to"].(string); ok {
				to = t
			}

			if bStr, ok := payload["Body"].(string); ok {
				body = bStr
			} else if bStr, ok := payload["body"].(string); ok {
				body = bStr
			}
		} else {
			return nil, fmt.Errorf("failed to parse twilio json: %w", err)
		}
	} else {
		if err := req.ParseForm(); err != nil {
			return nil, fmt.Errorf("failed to parse twilio form: %w", err)
		}
		from = req.FormValue("From")
		to = req.FormValue("To")
		body = req.FormValue("Body")
		raw, _ = json.Marshal(req.Form)
	}

	log.Printf("[DEBUG Twilio Webhook] Extracted -> From: %s, To: %s, Body: %s", from, to, body)

	runID := "unknown"
	if strings.TrimSpace(body) != "" {
		runID = strings.TrimSpace(body)
	}

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
