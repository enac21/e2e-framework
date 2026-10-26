package providers

import (
	"net/http"
	"strings"
	"time"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/pkg/httputil"
	"e2e-framework/internal/pkg/logger"
)

type TwilioExtractor struct {
	logger *logger.Logger
}

func NewTwilioExtractor() *TwilioExtractor {
	return &TwilioExtractor{
		logger: logger.New("providers").With(logger.AttrProvider, "twilio"),
	}
}

func (e *TwilioExtractor) Extract(req *http.Request) (*domain.Message, error) {
	fields, raw, err := httputil.ExtractFields(req)
	if err != nil {
		return nil, err
	}

	e.logger.Debug().Int("field_count", len(fields)).Msg("webhook fields extracted")

	runID := strings.TrimSpace(fields["body"])

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
