package imap_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"e2e-framework/internal/adapters/secondary/receiver/imap"
	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports/mocks"
)

func validOptions() map[string]string {
	return map[string]string{
		"host":     "imap.example.com",
		"port":     "993",
		"username": "user@example.com",
		"password": "secret",
		"mailbox":  "INBOX",
		"tls":      "true",
	}
}

func TestNewIMAPReceiver_MissingHost(t *testing.T) {
	opts := validOptions()
	delete(opts, "host")

	_, err := imap.NewIMAPReceiver(opts)

	if !errors.Is(err, domain.ErrConfiguration) {
		t.Fatalf("expected ErrConfiguration, got %v", err)
	}
}

func TestNewIMAPReceiver_EmptyHost(t *testing.T) {
	opts := validOptions()
	opts["host"] = ""

	_, err := imap.NewIMAPReceiver(opts)

	if !errors.Is(err, domain.ErrConfiguration) {
		t.Fatalf("expected ErrConfiguration, got %v", err)
	}
}

func TestNewIMAPReceiver_MissingPort(t *testing.T) {
	opts := validOptions()
	delete(opts, "port")

	_, err := imap.NewIMAPReceiver(opts)

	if !errors.Is(err, domain.ErrConfiguration) {
		t.Fatalf("expected ErrConfiguration, got %v", err)
	}
}

func TestNewIMAPReceiver_EmptyPort(t *testing.T) {
	opts := validOptions()
	opts["port"] = ""

	_, err := imap.NewIMAPReceiver(opts)

	if !errors.Is(err, domain.ErrConfiguration) {
		t.Fatalf("expected ErrConfiguration, got %v", err)
	}
}

func TestNewIMAPReceiver_InvalidPort(t *testing.T) {
	opts := validOptions()
	opts["port"] = "not-a-number"

	_, err := imap.NewIMAPReceiver(opts)

	if !errors.Is(err, domain.ErrConfiguration) {
		t.Fatalf("expected ErrConfiguration, got %v", err)
	}
}

func TestNewIMAPReceiver_DefaultMailbox(t *testing.T) {
	opts := validOptions()
	delete(opts, "mailbox")

	r, err := imap.NewIMAPReceiver(opts)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if r == nil {
		t.Fatal("expected non-nil receiver")
	}
}

func TestNewIMAPReceiver_ValidOptions(t *testing.T) {
	r, err := imap.NewIMAPReceiver(validOptions())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if r == nil {
		t.Fatal("expected non-nil receiver")
	}
}

func TestStart_ConnectError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mocks.NewMockIMAPClient(ctrl)

	mockClient.EXPECT().Connect().Return(errors.New("dial failed"))

	r := &imap.IMAPReceiver{Client: mockClient}
	err := r.Start(context.Background(), "run-123")

	if !errors.Is(err, domain.ErrInternal) {
		t.Fatalf("expected ErrInternal, got %v", err)
	}
}

func TestStart_SetsRunID(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mocks.NewMockIMAPClient(ctrl)

	mockClient.EXPECT().Connect().Return(nil)

	r := &imap.IMAPReceiver{Client: mockClient}
	err := r.Start(context.Background(), "run-abc")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if r.RunID != "run-abc" {
		t.Fatalf("expected runID %q, got %q", "run-abc", r.RunID)
	}
}

func TestCollect_NotStarted(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mocks.NewMockIMAPClient(ctrl)

	r := &imap.IMAPReceiver{Client: mockClient}

	_, err := r.Collect(context.Background())

	if !errors.Is(err, domain.ErrConfiguration) {
		t.Fatalf("expected ErrConfiguration, got %v", err)
	}
}

func TestCollect_ContextCancelledBeforeFirstTick(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mocks.NewMockIMAPClient(ctrl)

	r := &imap.IMAPReceiver{Client: mockClient, RunID: "run-cancel"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.Collect(ctx)

	if !errors.Is(err, domain.ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
}

func TestCollect_ContextTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mocks.NewMockIMAPClient(ctrl)

	mockClient.EXPECT().
		SearchByRunID(gomock.Any(), "run-timeout").
		Return(nil, nil).
		AnyTimes()

	r := &imap.IMAPReceiver{Client: mockClient, RunID: "run-timeout"}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := r.Collect(ctx)

	if !errors.Is(err, domain.ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
}

func TestCollect_SearchError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mocks.NewMockIMAPClient(ctrl)

	mockClient.EXPECT().
		SearchByRunID(gomock.Any(), "run-err").
		Return(nil, errors.New("connection reset"))

	r := &imap.IMAPReceiver{Client: mockClient, RunID: "run-err"}

	_, err := r.Collect(context.Background())

	if !errors.Is(err, domain.ErrInternal) {
		t.Fatalf("expected ErrInternal, got %v", err)
	}
}

func TestCollect_ReturnsMessageOnFirstSearch(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mocks.NewMockIMAPClient(ctrl)

	expected := &domain.Message{RunID: "run-found", ReceiverType: domain.ImapReceiverType}

	mockClient.EXPECT().
		SearchByRunID(gomock.Any(), "run-found").
		Return(expected, nil)

	r := &imap.IMAPReceiver{Client: mockClient, RunID: "run-found"}

	msg, err := r.Collect(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg != expected {
		t.Fatalf("expected message %v, got %v", expected, msg)
	}
}

func TestCollect_ReturnsMessageAfterRetries(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mocks.NewMockIMAPClient(ctrl)

	expected := &domain.Message{RunID: "run-retry", ReceiverType: domain.ImapReceiverType}

	gomock.InOrder(
		mockClient.EXPECT().SearchByRunID(gomock.Any(), "run-retry").Return(nil, nil),
		mockClient.EXPECT().SearchByRunID(gomock.Any(), "run-retry").Return(nil, nil),
		mockClient.EXPECT().SearchByRunID(gomock.Any(), "run-retry").Return(expected, nil),
	)

	r := &imap.IMAPReceiver{Client: mockClient, RunID: "run-retry"}

	msg, err := r.Collect(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg != expected {
		t.Fatalf("expected message %v, got %v", expected, msg)
	}
}

func TestStop_CallsDisconnect(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mocks.NewMockIMAPClient(ctrl)

	mockClient.EXPECT().Disconnect().Return(nil)

	r := &imap.IMAPReceiver{Client: mockClient}

	err := r.Stop()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStop_PropagatesDisconnectError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mocks.NewMockIMAPClient(ctrl)

	mockClient.EXPECT().Disconnect().Return(errors.New("logout failed"))

	r := &imap.IMAPReceiver{Client: mockClient}

	err := r.Stop()

	if err == nil {
		t.Fatal("expected error from Disconnect, got nil")
	}
}
