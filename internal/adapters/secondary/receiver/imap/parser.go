package imap

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"strings"
	"time"

	"github.com/emersion/go-message/mail"

	"e2e-framework/internal/core/domain"
)

func ParseMessage(runID string, raw []byte) (*domain.Message, error) {
	r, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse email: %v", domain.ErrInternal, err)
	}

	header := r.Header

	subject, _ := header.Subject()
	from := extractAddress(header, "From")
	to := extractAddress(header, "To")
	date, _ := header.Date()

	fields := map[string]string{
		"subject": subject,
		"from":    from,
		"to":      to,
		"date":    date.UTC().Format(time.RFC3339),
	}

	headers := extractRawHeaders(header)

	for {
		part, err := r.NextPart()
		if err == io.EOF {
			break
		}

		if err != nil {
			break
		}

		contentType, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		partBody, readErr := io.ReadAll(part.Body)
		if readErr != nil {
			continue
		}

		switch contentType {
		case "text/plain":
			if fields["body"] == "" {
				fields["body"] = strings.TrimSpace(string(partBody))
			}
		case "text/html":
			if fields["html_body"] == "" {
				fields["html_body"] = strings.TrimSpace(string(partBody))
			}
		}
	}

	return &domain.Message{
		RunID:        runID,
		ReceiverType: domain.ImapReceiverType,
		ReceivedAt:   time.Now().UTC(),
		Headers:      headers,
		Fields:       fields,
		Raw:          raw,
	}, nil
}

func extractAddress(h mail.Header, field string) string {
	addresses, err := h.AddressList(field)
	if err != nil || len(addresses) == 0 {
		return ""
	}

	addr := addresses[0]
	if addr.Name != "" {
		return fmt.Sprintf("%s <%s>", addr.Name, addr.Address)
	}

	return addr.Address
}

func extractRawHeaders(h mail.Header) map[string]string {
	result := make(map[string]string)

	fields := h.Fields()
	for fields.Next() {
		key := fields.Key()
		val, err := fields.Text()
		if err != nil {
			continue
		}

		if _, exists := result[key]; !exists {
			result[key] = val
		}
	}

	return result
}
