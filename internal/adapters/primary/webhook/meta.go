package webhook

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"e2e-framework/internal/core/domain"
)

type MetaExtractor struct{}

func NewMetaExtractor() Extractor {
	return &MetaExtractor{}
}

func (e *MetaExtractor) Extract(req *http.Request) (*domain.Message, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read meta body: %w", err)
	}
	defer req.Body.Close()

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}

	// Simplified logic for MVP. Assume WhatsApp or Messenger JSON struct
	// e.g. entry[0].changes[0].value.messages[0].text.body
	
	msg := &domain.Message{
		RunID:        "unknown", // To be extracted from body
		ReceiverType: "push",
		ReceivedAt:   time.Now(),
		Headers:      make(map[string]string),
		Fields:       make(map[string]string),
		Raw:          body,
	}

	if msgList, ok := payload["messages"].([]any); ok && len(msgList) > 0 {
		if firstMsg, ok := msgList[0].(map[string]any); ok {
			if textObj, ok := firstMsg["text"].(map[string]any); ok {
				if bodyText, ok := textObj["body"].(string); ok {
					msg.Fields["body"] = bodyText
					// If the body contains the runID
					if len(bodyText) >= 19 && bodyText[:3] == "ID:" {
						msg.RunID = bodyText[3:19]
					}
				}
			}
		}
	}

	return msg, nil
}
