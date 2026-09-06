package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"e2e-framework/internal/adapters/primary/api"
	receiverasserts "e2e-framework/internal/adapters/secondary/assertions/receiver"
	"e2e-framework/internal/adapters/secondary/receiver"
	"e2e-framework/internal/adapters/secondary/trigger"
	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
	"e2e-framework/internal/core/ports/mocks"
	"e2e-framework/internal/core/services"
	"e2e-framework/internal/pkg/config"
)

func newTestServer(
	t *testing.T,
	ctrl *gomock.Controller,
	tests map[string]domain.TestDefinition,
	groups ...map[string]config.TestGroupConfig,
) (*api.Server, *mocks.MockTrigger, *mocks.MockNotifier) {
	t.Helper()

	mockTrigger := mocks.NewMockTrigger(ctrl)
	mockStore := mocks.NewMockStore(ctrl)
	mockNotifier := mocks.NewMockNotifier(ctrl)

	triggerReg := trigger.NewTriggerRegistry()
	triggerReg.Register(domain.HTTPTriggerType, func(options map[string]string) (ports.Trigger, error) {
		return mockTrigger, nil
	})

	orch := services.NewOrchestrator(
		triggerReg,
		mockStore,
		receiver.NewReceiverRegistry(),
		receiverasserts.NewReceiverAssertionRegistry(),
		mockNotifier,
	)

	var resolver ports.GroupResolver
	if len(groups) > 0 {
		resolver = services.NewGroupResolver(groups[0])
	}

	srv := api.NewServer(&api.Config{AuthEnable: false, Resolver: resolver}, orch, tests)

	return srv, mockTrigger, mockNotifier
}

func postSequence(srv *api.Server, target string, rules []string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(rules)
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Mux().ServeHTTP(w, req)

	return w
}

func postBody(srv *api.Server, target string, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Mux().ServeHTTP(w, req)

	return w
}

func TestHandleRunSequence_MethodNotAllowed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	srv, _, _ := newTestServer(t, ctrl, map[string]domain.TestDefinition{})
	req := httptest.NewRequest(http.MethodGet, "/run-sequence", nil)
	w := httptest.NewRecorder()
	srv.Mux().ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleRunSequence_InvalidJSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	srv, _, _ := newTestServer(t, ctrl, map[string]domain.TestDefinition{})
	req := httptest.NewRequest(http.MethodPost, "/run-sequence", strings.NewReader("{invalid}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Mux().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleRunSequence_EmptyRules(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	srv, _, _ := newTestServer(t, ctrl, map[string]domain.TestDefinition{})
	w := postSequence(srv, "/run-sequence", []string{})

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleRunSequence_TestNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := map[string]domain.TestDefinition{
		"real": {ID: "real", Enabled: true},
	}
	srv, _, _ := newTestServer(t, ctrl, tests)
	w := postSequence(srv, "/run-sequence", []string{"nonexistent"})

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleRunSequence_InvalidDelay(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := map[string]domain.TestDefinition{
		"t1": {ID: "t1", Enabled: true},
	}
	srv, _, _ := newTestServer(t, ctrl, tests)
	w := postSequence(srv, "/run-sequence?test_delay=notaduration", []string{"t1"})

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleRunSequence_SingleTest_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := map[string]domain.TestDefinition{
		"t1": {ID: "t1", Enabled: true},
	}
	srv, _, _ := newTestServer(t, ctrl, tests)
	w := postSequence(srv, "/run-sequence", []string{"t1"})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var results []*domain.TestResult
	if err := json.NewDecoder(w.Body).Decode(&results); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].TestID != "t1" {
		t.Errorf("expected TestID %q, got %q", "t1", results[0].TestID)
	}
}

func TestHandleRunSequence_MultipleTests_ReturnsAllResults(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := map[string]domain.TestDefinition{
		"a": {ID: "a", Enabled: true},
		"b": {ID: "b", Enabled: true},
	}
	srv, _, _ := newTestServer(t, ctrl, tests)
	w := postSequence(srv, "/run-sequence", []string{"a", "b"})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var results []*domain.TestResult
	if err := json.NewDecoder(w.Body).Decode(&results); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestHandleRunSequence_ResultsStoredInServer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := map[string]domain.TestDefinition{
		"t1": {ID: "t1", Enabled: true},
	}
	srv, _, _ := newTestServer(t, ctrl, tests)
	w := postSequence(srv, "/run-sequence", []string{"t1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var results []*domain.TestResult
	if err := json.NewDecoder(w.Body).Decode(&results); err != nil {
		t.Fatalf("failed to decode sequence response: %v", err)
	}

	runID := results[0].RunID
	req := httptest.NewRequest(http.MethodGet, "/results/"+runID, nil)
	wr := httptest.NewRecorder()
	srv.Mux().ServeHTTP(wr, req)

	if wr.Code != http.StatusOK {
		t.Errorf("expected stored result GET /results/%s to return 200, got %d", runID, wr.Code)
	}
}

func TestHandleRunSequence_WithDelay_DoesNotTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := map[string]domain.TestDefinition{
		"t1": {ID: "t1", Enabled: true},
	}
	srv, _, _ := newTestServer(t, ctrl, tests)
	w := postSequence(srv, "/run-sequence?test_delay=10ms", []string{"t1"})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleRunSequence_SkipFailTest_ReturnsPartialResults(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	failTrigger := domain.TriggerConfig{Method: "POST", URL: "http://x"}
	tests := map[string]domain.TestDefinition{
		"fail": {ID: "fail", Enabled: true, Triggers: []domain.TriggerConfig{failTrigger}},
		"ok":   {ID: "ok", Enabled: true},
	}
	srv, mockTrigger, mockNotifier := newTestServer(t, ctrl, tests)

	notified := make(chan struct{})

	mockTrigger.EXPECT().
		Execute(gomock.Any(), failTrigger, gomock.Any(), gomock.Any()).
		Return(nil, errors.New("trigger error"))
	mockNotifier.EXPECT().
		Notify(gomock.Any(), gomock.Any(), gomock.Any()).
		Do(func(context.Context, domain.OnFailureConfig, *domain.TestResult) { close(notified) }).
		Return(nil)

	w := postSequence(srv, "/run-sequence?skip_fail_test=true", []string{"fail", "ok"})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var results []*domain.TestResult
	if err := json.NewDecoder(w.Body).Decode(&results); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result (stopped after failure), got %d", len(results))
	}

	<-notified
}

func TestHandleRunSequence_SkipFailTest_False_ReturnsAllResults(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	failTrigger := domain.TriggerConfig{Method: "POST", URL: "http://x"}
	tests := map[string]domain.TestDefinition{
		"fail": {ID: "fail", Enabled: true, Triggers: []domain.TriggerConfig{failTrigger}},
		"ok":   {ID: "ok", Enabled: true},
	}
	srv, mockTrigger, mockNotifier := newTestServer(t, ctrl, tests)

	notified := make(chan struct{})

	mockTrigger.EXPECT().
		Execute(gomock.Any(), failTrigger, gomock.Any(), gomock.Any()).
		Return(nil, errors.New("trigger error"))
	mockNotifier.EXPECT().
		Notify(gomock.Any(), gomock.Any(), gomock.Any()).
		Do(func(context.Context, domain.OnFailureConfig, *domain.TestResult) { close(notified) }).
		Return(nil)

	w := postSequence(srv, "/run-sequence?skip_fail_test=false", []string{"fail", "ok"})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var results []*domain.TestResult
	if err := json.NewDecoder(w.Body).Decode(&results); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results (continues after failure), got %d", len(results))
	}

	<-notified
}

func TestHandleRunSequence_TestGroup_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := map[string]domain.TestDefinition{
		"a": {ID: "a", Enabled: true},
		"b": {ID: "b", Enabled: true},
	}
	groups := map[string]config.TestGroupConfig{
		"ci": {Tests: []string{"a", "b"}, TestDelay: 2 * time.Second, SkipFailTest: true},
	}
	srv, _, _ := newTestServer(t, ctrl, tests, groups)
	w := postBody(srv, "/run-sequence?test_group=ci", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var results []*domain.TestResult
	if err := json.NewDecoder(w.Body).Decode(&results); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestHandleRunSequence_TestGroup_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	srv, _, _ := newTestServer(t, ctrl, map[string]domain.TestDefinition{})
	w := postBody(srv, "/run-sequence?test_group=nope", "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleRunSequence_TestGroupAndBodyList_MutuallyExclusive(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := map[string]domain.TestDefinition{
		"a": {ID: "a", Enabled: true},
	}
	groups := map[string]config.TestGroupConfig{
		"ci": {Tests: []string{"a"}},
	}
	srv, _, _ := newTestServer(t, ctrl, tests, groups)
	w := postBody(srv, "/run-sequence?test_group=ci", `["a"]`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleRunSequence_TestGroup_QueryOverridesGroupDefaults(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	failTrigger := domain.TriggerConfig{Method: "POST", URL: "http://x"}
	tests := map[string]domain.TestDefinition{
		"fail": {ID: "fail", Enabled: true, Triggers: []domain.TriggerConfig{failTrigger}},
		"ok":   {ID: "ok", Enabled: true},
	}
	groups := map[string]config.TestGroupConfig{
		"ci": {Tests: []string{"fail", "ok"}, SkipFailTest: true},
	}
	srv, mockTrigger, mockNotifier := newTestServer(t, ctrl, tests, groups)

	notified := make(chan struct{})

	mockTrigger.EXPECT().
		Execute(gomock.Any(), failTrigger, gomock.Any(), gomock.Any()).
		Return(nil, errors.New("trigger error"))
	mockNotifier.EXPECT().
		Notify(gomock.Any(), gomock.Any(), gomock.Any()).
		Do(func(context.Context, domain.OnFailureConfig, *domain.TestResult) { close(notified) }).
		Return(nil)

	w := postBody(srv, "/run-sequence?test_group=ci&skip_fail_test=false", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var results []*domain.TestResult
	if err := json.NewDecoder(w.Body).Decode(&results); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results (query param overrode group default), got %d", len(results))
	}

	<-notified
}
