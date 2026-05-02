package domain

import "errors"

// Core domain errors to be wrapped by adapters and evaluated by the orchestrator.
var (
	ErrConfiguration = errors.New("configuration error")
	ErrTriggerFailed = errors.New("trigger failed")
	ErrTimeout       = errors.New("timeout exceeded")
	ErrNotFound      = errors.New("not found")
	ErrValidation    = errors.New("validation failed")
	ErrInternal      = errors.New("internal system error")
	ErrUnimplemented = errors.New("unimplemented feature")
)
