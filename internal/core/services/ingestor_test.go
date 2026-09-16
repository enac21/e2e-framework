package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports/mocks"
)

func TestNewIngestor_NilStore(t *testing.T) {
	_, err := NewIngestor(nil)
	if !errors.Is(err, domain.ErrConfiguration) {
		t.Fatalf("want ErrConfiguration, got %v", err)
	}
}

func TestIngestor_Ingest_Validation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := mocks.NewMockStore(ctrl)
	ingestor, _ := NewIngestor(mockStore)

	if err := ingestor.Ingest(context.Background(), nil); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation for nil msg, got %v", err)
	}

	if err := ingestor.Ingest(context.Background(), &domain.Message{RunID: ""}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation for empty run_id, got %v", err)
	}

	if err := ingestor.Ingest(context.Background(), &domain.Message{RunID: "unknown"}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation for unknown run_id, got %v", err)
	}
}

func TestIngestor_Ingest_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := mocks.NewMockStore(ctrl)
	ingestor, _ := NewIngestor(mockStore)

	msg := &domain.Message{RunID: "run-123", ReceiverType: domain.WebhookReceiverType}
	mockStore.EXPECT().Deposit(gomock.Any(), msg).Return(nil)

	if err := ingestor.Ingest(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.ReceivedAt.IsZero() {
		t.Fatal("expected ReceivedAt to be set")
	}
}

func TestIngestor_Ingest_StoreError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := mocks.NewMockStore(ctrl)
	ingestor, _ := NewIngestor(mockStore)

	msg := &domain.Message{RunID: "run-123"}
	mockStore.EXPECT().Deposit(gomock.Any(), msg).Return(errors.New("deposit failed"))

	err := ingestor.Ingest(context.Background(), msg)
	if !errors.Is(err, domain.ErrInternal) {
		t.Fatalf("want ErrInternal, got %v", err)
	}
}

func TestIngestor_Ingest_PreservesReceivedAt(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := mocks.NewMockStore(ctrl)
	ingestor, _ := NewIngestor(mockStore)

	fixed := time.Now().Add(-time.Hour)
	msg := &domain.Message{RunID: "run-123", ReceivedAt: fixed}
	mockStore.EXPECT().Deposit(gomock.Any(), msg).Return(nil)

	if err := ingestor.Ingest(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !msg.ReceivedAt.Equal(fixed) {
		t.Fatalf("expected ReceivedAt preserved, got %v want %v", msg.ReceivedAt, fixed)
	}
}
