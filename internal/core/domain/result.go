package domain

import "time"

type RunStatus string

const (
	StatusPassed  RunStatus = "passed"
	StatusFailed  RunStatus = "failed"
	StatusError   RunStatus = "error"
	StatusSkipped RunStatus = "skipped"
)

type ReceiverResult struct {
	Type       string
	Status     RunStatus
	DurationMs int64
	Error      string
	Message    *Message
}

type TestResult struct {
	TestID     string
	RunID      string
	Status     RunStatus
	DurationMs int64
	Receivers  []ReceiverResult
	Error      string
	StartedAt  time.Time
	FinishedAt time.Time
}
