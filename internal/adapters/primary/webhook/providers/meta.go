package providers

import (
	"net/http"
	"time"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/pkg/httputil"
	"e2e-framework/internal/pkg/logger"
)

type MetaExtractor struct {
	logger *logger.Logger
}

func NewMetaExtractor() *MetaExtractor {
	return &MetaExtractor{
		logger: logger.New("providers").With(logger.AttrProvider, "meta"),
	}
}

func (e *MetaExtractor) Extract(req *http.Request) (*domain.Message, error) {
	fields, raw, err := httputil.ExtractFields(req)
	if err != nil {
		return nil, err
	}

	e.logger.Debug().Int("field_count", len(fields)).Msg("webhook fields extracted")

	runID := fields["messages.0.text.body"]
	if runID == "" {
		runID = "unknown"
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
