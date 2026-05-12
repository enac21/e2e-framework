package imap_test

import (
	"strings"
	"testing"

	"e2e-framework/internal/adapters/secondary/receiver/imap"
	"e2e-framework/internal/core/domain"
)

var plainTextRaw = []byte("From: Sender Name <sender@example.com>\r\n" +
	"To: receiver@example.com\r\n" +
	"Subject: Welcome run-abc-123\r\n" +
	"Date: Sat, 10 May 2026 18:00:00 +0000\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"Hello, your run ID is run-abc-123. Welcome!\r\n")

var multipartRaw = []byte("From: sender@example.com\r\n" +
	"To: receiver@example.com\r\n" +
	"Subject: Hello\r\n" +
	"Date: Sat, 10 May 2026 18:00:00 +0000\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: multipart/alternative; boundary=\"boundary42\"\r\n" +
	"\r\n" +
	"--boundary42\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"Plain body here.\r\n" +
	"--boundary42\r\n" +
	"Content-Type: text/html; charset=utf-8\r\n" +
	"\r\n" +
	"<p>HTML body here.</p>\r\n" +
	"--boundary42--\r\n")

var htmlOnlyRaw = []byte("From: sender@example.com\r\n" +
	"To: receiver@example.com\r\n" +
	"Subject: HTML Only\r\n" +
	"Date: Sat, 10 May 2026 18:00:00 +0000\r\n" +
	"Content-Type: text/html; charset=utf-8\r\n" +
	"\r\n" +
	"<p>Only HTML here.</p>\r\n")

var encodedSubjectRaw = []byte("From: sender@example.com\r\n" +
	"To: receiver@example.com\r\n" +
	"Subject: =?utf-8?q?Bienvenido_run-xyz?=\r\n" +
	"Date: Sat, 10 May 2026 18:00:00 +0000\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"body\r\n")

var namedSenderRaw = []byte("From: John Doe <john@example.com>\r\n" +
	"To: Jane Doe <jane@example.com>\r\n" +
	"Subject: Named Sender Test\r\n" +
	"Date: Sat, 10 May 2026 18:00:00 +0000\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"body\r\n")

func assertField(t *testing.T, msg *domain.Message, key, expected string) {
	t.Helper()

	got, ok := msg.Fields[key]
	if !ok {
		t.Fatalf("field %q not present", key)
	}

	if got != expected {
		t.Fatalf("field %q: expected %q, got %q", key, expected, got)
	}
}

func TestParseMessage_PlainText_PopulatesAllFields(t *testing.T) {
	msg, err := imap.ParseMessage("run-abc-123", plainTextRaw)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.RunID != "run-abc-123" {
		t.Fatalf("expected RunID %q, got %q", "run-abc-123", msg.RunID)
	}

	if msg.ReceiverType != domain.ImapReceiverType {
		t.Fatalf("expected ReceiverType %q, got %q", domain.ImapReceiverType, msg.ReceiverType)
	}

	if msg.ReceivedAt.IsZero() {
		t.Fatal("expected non-zero ReceivedAt")
	}

	if len(msg.Raw) == 0 {
		t.Fatal("expected non-empty Raw bytes")
	}

	assertField(t, msg, "subject", "Welcome run-abc-123")
	assertField(t, msg, "to", "receiver@example.com")

	if msg.Fields["body"] == "" {
		t.Fatal("expected non-empty body")
	}
}

func TestParseMessage_PlainText_FromWithName(t *testing.T) {
	msg, err := imap.ParseMessage("run-x", namedSenderRaw)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	from := msg.Fields["from"]
	if !strings.Contains(from, "john@example.com") {
		t.Fatalf("expected from to contain email address, got %q", from)
	}
}

func TestParseMessage_Multipart_ExtractsBothParts(t *testing.T) {
	msg, err := imap.ParseMessage("run-mp", multipartRaw)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.Fields["body"] == "" {
		t.Fatal("expected non-empty body (text/plain part)")
	}

	if msg.Fields["html_body"] == "" {
		t.Fatal("expected non-empty html_body (text/html part)")
	}
}

func TestParseMessage_HTMLOnly_EmptyBody(t *testing.T) {
	msg, err := imap.ParseMessage("run-html", htmlOnlyRaw)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.Fields["body"] != "" {
		t.Fatalf("expected empty body, got %q", msg.Fields["body"])
	}

	if msg.Fields["html_body"] == "" {
		t.Fatal("expected non-empty html_body")
	}
}

func TestParseMessage_EncodedSubject_DecodedCorrectly(t *testing.T) {
	msg, err := imap.ParseMessage("run-xyz", encodedSubjectRaw)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertField(t, msg, "subject", "Bienvenido run-xyz")
}

func TestParseMessage_HeadersPopulated(t *testing.T) {
	msg, err := imap.ParseMessage("run-h", plainTextRaw)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msg.Headers) == 0 {
		t.Fatal("expected non-empty Headers map")
	}

	if _, ok := msg.Headers["Subject"]; !ok {
		t.Fatal("expected 'Subject' in Headers")
	}
}

func TestParseMessage_DateFieldPopulated(t *testing.T) {
	msg, err := imap.ParseMessage("run-d", plainTextRaw)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.Fields["date"] == "" {
		t.Fatal("expected non-empty date field")
	}
}

func TestParseMessage_MalformedEmail_ReturnsError(t *testing.T) {
	_, err := imap.ParseMessage("run-bad", []byte("this is not an email at all @@@@"))

	if err == nil {
		t.Fatal("expected error for malformed email, got nil")
	}
}
