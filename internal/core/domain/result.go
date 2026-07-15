package domain

import "time"

type RunStatus = string

const (
	StatusPassed  RunStatus = "passed"
	StatusFailed  RunStatus = "failed"
	StatusError   RunStatus = "error"
	StatusSkipped RunStatus = "skipped"
	StatusRunning RunStatus = "running"
)

type ReceiverResult struct {
	Type         string    `json:"type"`
	TriggerIndex int       `json:"trigger_index"`
	Status       RunStatus `json:"status"`
	DurationMs   int64     `json:"duration_ms"`
	Error        string    `json:"error,omitempty"`
	Message      *Message  `json:"message,omitempty"`
}

type TestResult struct {
	TestID      string            `json:"test_id"`
	RunID       string            `json:"run_id"`
	Status      RunStatus         `json:"status"`
	Attempts    int               `json:"attempts"`
	DurationMs  int64             `json:"duration_ms"`
	Receivers   []ReceiverResult  `json:"receivers,omitempty"`
	Error       string            `json:"error,omitempty"`
	StartedAt   time.Time         `json:"started_at"`
	FinishedAt  time.Time         `json:"finished_at"`
	TriggerVars map[string]string `json:"trigger_vars,omitempty"`
}
