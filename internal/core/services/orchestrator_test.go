package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	receiverasserts "e2e-framework/internal/adapters/secondary/assertions/receiver"
	"e2e-framework/internal/adapters/secondary/receiver"
	"e2e-framework/internal/adapters/secondary/trigger"
	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
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

	triggerReg := trigger.NewTriggerRegistry()
	triggerReg.Register(domain.HTTPTriggerType, func(options map[string]string) (ports.Trigger, error) {
		return mockTrigger, nil
	})

	orch := NewOrchestrator(
		triggerReg,
		mockStore,
		receiver.NewReceiverRegistry(),
		receiverasserts.NewReceiverAssertionRegistry(),
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

	notified := make(chan struct{})

	mockTrigger.EXPECT().
		Execute(gomock.Any(), failTrigger, gomock.Any(), gomock.Any()).
		Return(nil, errors.New("trigger error"))
	mockNotifier.EXPECT().
		Notify(gomock.Any(), gomock.Any(), gomock.Any()).
		Do(func(context.Context, domain.OnFailureConfig, *domain.TestResult) { close(notified) }).
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

	<-notified
}

func TestRunSequence_SkipFailTest_False_ContinuesAfterFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orch, mockTrigger, _, mockNotifier := newTestOrchestrator(t, ctrl)

	failTrigger := domain.TriggerConfig{Method: "POST", URL: "http://x"}
	def1 := domain.TestDefinition{ID: "fail", Enabled: true, Triggers: []domain.TriggerConfig{failTrigger}}
	def2 := domain.TestDefinition{ID: "ok", Enabled: true}

	notified := make(chan struct{})

	mockTrigger.EXPECT().
		Execute(gomock.Any(), failTrigger, gomock.Any(), gomock.Any()).
		Return(nil, errors.New("trigger error"))
	mockNotifier.EXPECT().
		Notify(gomock.Any(), gomock.Any(), gomock.Any()).
		Do(func(context.Context, domain.OnFailureConfig, *domain.TestResult) { close(notified) }).
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

	<-notified
}

func TestVariables_StaticAliasInjectedIntoTriggerVars(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orch, mockTrigger, _, _ := newTestOrchestrator(t, ctrl)

	trig := domain.TriggerConfig{Method: "GET", URL: "http://x"}
	def := domain.TestDefinition{
		ID:        "t1",
		Enabled:   true,
		Variables: map[string]string{"base_url": "http://svc.local", "env_name": "prod"},
		Triggers:  []domain.TriggerConfig{trig},
	}

	var receivedVars map[string]string
	mockTrigger.EXPECT().
		Execute(gomock.Any(), trig, gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ domain.TriggerConfig, _ string, vars map[string]string) (map[string]string, error) {
			receivedVars = vars

			return map[string]string{}, nil
		}).
		Times(1)

	results := orch.RunSequence(context.Background(), []domain.TestDefinition{def}, SequenceConfig{})
	_ = results

	if receivedVars["base_url"] != "http://svc.local" {
		t.Errorf("expected base_url var injected, got %q", receivedVars["base_url"])
	}

	if receivedVars["env_name"] != "prod" {
		t.Errorf("expected env_name var injected, got %q", receivedVars["env_name"])
	}

	if receivedVars["run_id"] == "" {
		t.Error("expected run_id to remain present")
	}
}

func TestVariables_GeneratorEvaluatedOnceAcrossTriggers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orch, mockTrigger, _, _ := newTestOrchestrator(t, ctrl)

	trigA := domain.TriggerConfig{Method: "GET", URL: "http://a"}
	trigB := domain.TriggerConfig{Method: "GET", URL: "http://b"}
	def := domain.TestDefinition{
		ID:        "t1",
		Enabled:   true,
		Variables: map[string]string{"request_id": "{{uuid()}}"},
		Triggers:  []domain.TriggerConfig{trigA, trigB},
	}

	var firstRequestID, secondRequestID string
	mockTrigger.EXPECT().
		Execute(gomock.Any(), trigA, gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ domain.TriggerConfig, _ string, vars map[string]string) (map[string]string, error) {
			firstRequestID = vars["request_id"]

			return map[string]string{}, nil
		}).
		Times(1)
	mockTrigger.EXPECT().
		Execute(gomock.Any(), trigB, gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ domain.TriggerConfig, _ string, vars map[string]string) (map[string]string, error) {
			secondRequestID = vars["request_id"]

			return map[string]string{}, nil
		}).
		Times(1)

	orch.RunSequence(context.Background(), []domain.TestDefinition{def}, SequenceConfig{})

	if firstRequestID == "" || secondRequestID == "" {
		t.Fatalf("expected request_id generator to resolve, got first=%q second=%q", firstRequestID, secondRequestID)
	}

	if firstRequestID != secondRequestID {
		t.Errorf("expected request_id to be stable across triggers, got first=%q second=%q", firstRequestID, secondRequestID)
	}
}

func TestVariables_ExtractOverridesVariableWithSameName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orch, mockTrigger, _, _ := newTestOrchestrator(t, ctrl)

	trig := domain.TriggerConfig{Method: "GET", URL: "http://x", Extract: map[string]string{"user_id": "id"}}
	def := domain.TestDefinition{
		ID:        "t1",
		Enabled:   true,
		Variables: map[string]string{"user_id": "static-value"},
		Triggers:  []domain.TriggerConfig{trig},
	}

	mockTrigger.EXPECT().
		Execute(gomock.Any(), trig, gomock.Any(), gomock.Any()).
		Return(map[string]string{"user_id": "dynamic-42"}, nil)

	results := orch.RunSequence(context.Background(), []domain.TestDefinition{def}, SequenceConfig{})

	if got := results[0].TriggerVars["user_id"]; got != "dynamic-42" {
		t.Errorf("expected extract to override variable, got %q", got)
	}
}

func TestVariables_ReservedNamesIgnored(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orch, mockTrigger, _, _ := newTestOrchestrator(t, ctrl)

	trig := domain.TriggerConfig{Method: "GET", URL: "http://x"}
	def := domain.TestDefinition{
		ID:        "t1",
		Enabled:   true,
		Variables: map[string]string{"run_id": "overridden", "test_id": "overridden", "custom": "ok"},
		Triggers:  []domain.TriggerConfig{trig},
	}

	var receivedVars map[string]string
	mockTrigger.EXPECT().
		Execute(gomock.Any(), trig, gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ domain.TriggerConfig, runID string, vars map[string]string) (map[string]string, error) {
			receivedVars = vars

			return map[string]string{}, nil
		}).
		Times(1)

	orch.RunSequence(context.Background(), []domain.TestDefinition{def}, SequenceConfig{})

	if receivedVars["run_id"] == "overridden" {
		t.Error("expected reserved name run_id not to be overridden by variables block")
	}

	if receivedVars["run_id"] == "" {
		t.Error("expected builtin run_id to remain set")
	}

	if _, ok := receivedVars["test_id"]; ok {
		t.Error("expected reserved name test_id not to be present from variables block")
	}

	if receivedVars["custom"] != "ok" {
		t.Errorf("expected non-reserved custom var to be injected, got %q", receivedVars["custom"])
	}
}

func TestRunSequence_TriggerVarsPopulatedOnFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orch, mockTrigger, _, mockNotifier := newTestOrchestrator(t, ctrl)

	okTrigger := domain.TriggerConfig{Method: "GET", URL: "http://ok", Extract: map[string]string{"transaction_id": "id"}}
	failTrigger := domain.TriggerConfig{Method: "GET", URL: "http://fail"}

	def := domain.TestDefinition{
		ID:      "t1",
		Enabled: true,
		Triggers: []domain.TriggerConfig{
			okTrigger,
			failTrigger,
		},
	}

	notified := make(chan struct{})

	mockTrigger.EXPECT().
		Execute(gomock.Any(), okTrigger, gomock.Any(), gomock.Any()).
		Return(map[string]string{"transaction_id": "txn-42"}, nil)
	mockTrigger.EXPECT().
		Execute(gomock.Any(), failTrigger, gomock.Any(), gomock.Any()).
		Return(nil, errors.New("trigger error"))
	mockNotifier.EXPECT().
		Notify(gomock.Any(), gomock.Any(), gomock.Any()).
		Do(func(context.Context, domain.OnFailureConfig, *domain.TestResult) { close(notified) }).
		Return(nil)

	results := orch.RunSequence(context.Background(), []domain.TestDefinition{def}, SequenceConfig{})
	result := results[0]

	if result.Status != domain.StatusError {
		t.Fatalf("expected status %q, got %q", domain.StatusError, result.Status)
	}

	if got := result.TriggerVars["transaction_id"]; got != "txn-42" {
		t.Errorf("expected extracted var preserved on failure, got %q", got)
	}

	<-notified
}

func TestRunSequence_UnknownTriggerTypeFailsStep(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orch, mockTrigger, _, mockNotifier := newTestOrchestrator(t, ctrl)

	def := domain.TestDefinition{
		ID:      "t1",
		Enabled: true,
		Triggers: []domain.TriggerConfig{
			{Type: "smtp"},
		},
	}

	mockTrigger.EXPECT().
		Execute(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	mockNotifier.EXPECT().
		Notify(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		MinTimes(0)

	results := orch.RunSequence(context.Background(), []domain.TestDefinition{def}, SequenceConfig{})
	result := results[0]

	if result.Status != domain.StatusError {
		t.Fatalf("expected status %q, got %q", domain.StatusError, result.Status)
	}

	if !strings.Contains(result.Error, "unknown trigger type") {
		t.Errorf("expected error to mention unknown trigger type, got %q", result.Error)
	}
}
