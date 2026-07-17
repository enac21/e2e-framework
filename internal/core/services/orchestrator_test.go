package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"e2e-framework/internal/adapters/secondary/assertion"
	"e2e-framework/internal/adapters/secondary/receiver"
	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports/mocks"
)

func newTestOrchestrator(
	t *testing.T,
	ctrl *gomock.Controller,
) (*Orchestrator, *mocks.MockTrigger, *mocks.MockStore, *mocks.MockNotifier) {
	t.Helper()

	mockTrigger := mocks.NewMockTrigger(ctrl)
	mockStore := mocks.NewMockStore(ctrl)
	mockNotifier := mocks.NewMockNotifier(ctrl)

	orch := NewOrchestrator(
		mockTrigger,
		mockStore,
		receiver.NewReceiverRegistry(),
		assertion.NewAssertionRegistry(),
		mockNotifier,
	)

	return orch, mockTrigger, mockStore, mockNotifier
}

func TestRunSequence_EmptyRules(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orch, _, _, _ := newTestOrchestrator(t, ctrl)
	results := orch.RunSequence(context.Background(), []domain.TestDefinition{}, SequenceConfig{})

	if results == nil {
		t.Fatal("expected non-nil slice")
	}

	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestRunSequence_SingleTest_Passed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orch, _, _, _ := newTestOrchestrator(t, ctrl)
	defs := []domain.TestDefinition{{ID: "t1", Enabled: true}}

	results := orch.RunSequence(context.Background(), defs, SequenceConfig{})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].TestID != "t1" {
		t.Errorf("expected TestID %q, got %q", "t1", results[0].TestID)
	}

	if results[0].Status != domain.StatusPassed {
		t.Errorf("expected status %q, got %q", domain.StatusPassed, results[0].Status)
	}
}

func TestRunSequence_SingleTest_Disabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orch, _, _, _ := newTestOrchestrator(t, ctrl)
	defs := []domain.TestDefinition{{ID: "disabled", Enabled: false}}

	results := orch.RunSequence(context.Background(), defs, SequenceConfig{})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Status != domain.StatusSkipped {
		t.Errorf("expected status %q, got %q", domain.StatusSkipped, results[0].Status)
	}
}

func TestRunSequence_MultipleTests_RunInOrder(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orch, _, _, _ := newTestOrchestrator(t, ctrl)
	defs := []domain.TestDefinition{
		{ID: "first", Enabled: true},
		{ID: "second", Enabled: true},
	}

	results := orch.RunSequence(context.Background(), defs, SequenceConfig{})

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].TestID != "first" {
		t.Errorf("expected first result TestID %q, got %q", "first", results[0].TestID)
	}

	if results[1].TestID != "second" {
		t.Errorf("expected second result TestID %q, got %q", "second", results[1].TestID)
	}
}

func TestRunSequence_DelayAppliedBetweenTests(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orch, _, _, _ := newTestOrchestrator(t, ctrl)
	defs := []domain.TestDefinition{
		{ID: "a", Enabled: true},
		{ID: "b", Enabled: true},
	}

	delay := 10 * time.Millisecond
	start := time.Now()
	orch.RunSequence(context.Background(), defs, SequenceConfig{Delay: delay})
	elapsed := time.Since(start)

	if elapsed < delay {
		t.Errorf("expected elapsed >= %v (delay between tests), got %v", delay, elapsed)
	}
}

func TestRunSequence_NoDelayBeforeFirstTest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orch, _, _, _ := newTestOrchestrator(t, ctrl)
	defs := []domain.TestDefinition{{ID: "only", Enabled: true}}

	delay := 50 * time.Millisecond
	start := time.Now()
	orch.RunSequence(context.Background(), defs, SequenceConfig{Delay: delay})
	elapsed := time.Since(start)

	if elapsed >= delay {
		t.Errorf("expected elapsed < %v (no delay before first test), got %v", delay, elapsed)
	}
}

func TestRunSequence_StoresRunIDInResult(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orch, _, _, _ := newTestOrchestrator(t, ctrl)
	defs := []domain.TestDefinition{{ID: "x", Enabled: true}}

	results := orch.RunSequence(context.Background(), defs, SequenceConfig{})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].RunID == "" {
		t.Error("expected non-empty RunID")
	}
}

func TestRunSequence_SkipFailTest_StopsAfterFirstFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orch, mockTrigger, _, mockNotifier := newTestOrchestrator(t, ctrl)

	failTrigger := domain.TriggerConfig{Method: "POST", URL: "http://x"}
	def1 := domain.TestDefinition{ID: "fail", Enabled: true, Triggers: []domain.TriggerConfig{failTrigger}}
	def2 := domain.TestDefinition{ID: "skip", Enabled: true}

	mockTrigger.EXPECT().
		Execute(gomock.Any(), failTrigger, gomock.Any(), gomock.Any()).
		Return(nil, errors.New("trigger error"))
	mockNotifier.EXPECT().
		Notify(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	results := orch.RunSequence(
		context.Background(),
		[]domain.TestDefinition{def1, def2},
		SequenceConfig{SkipFailTest: true},
	)

	if len(results) != 1 {
		t.Fatalf("expected 1 result (stopped after failure), got %d", len(results))
	}

	if results[0].Status != domain.StatusError {
		t.Errorf("expected status %q, got %q", domain.StatusError, results[0].Status)
	}
}

func TestRunSequence_SkipFailTest_False_ContinuesAfterFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orch, mockTrigger, _, mockNotifier := newTestOrchestrator(t, ctrl)

	failTrigger := domain.TriggerConfig{Method: "POST", URL: "http://x"}
	def1 := domain.TestDefinition{ID: "fail", Enabled: true, Triggers: []domain.TriggerConfig{failTrigger}}
	def2 := domain.TestDefinition{ID: "ok", Enabled: true}

	mockTrigger.EXPECT().
		Execute(gomock.Any(), failTrigger, gomock.Any(), gomock.Any()).
		Return(nil, errors.New("trigger error"))
	mockNotifier.EXPECT().
		Notify(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	results := orch.RunSequence(
		context.Background(),
		[]domain.TestDefinition{def1, def2},
		SequenceConfig{SkipFailTest: false},
	)

	if len(results) != 2 {
		t.Fatalf("expected 2 results (continues after failure), got %d", len(results))
	}

	if results[0].Status != domain.StatusError {
		t.Errorf("expected first result status %q, got %q", domain.StatusError, results[0].Status)
	}

	if results[1].Status != domain.StatusPassed {
		t.Errorf("expected second result status %q, got %q", domain.StatusPassed, results[1].Status)
	}
}
