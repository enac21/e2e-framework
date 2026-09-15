package ports

import (
	"context"

	"e2e-framework/internal/core/domain"
)

//go:generate go run -mod=mod go.uber.org/mock/mockgen -source=notifier.go -destination=mocks/mock_notifier.go -package=mocks

// Notifier executes the on_failure.calls configured for a failed test.
// Implementations are fire-and-forget: a non-nil error means the alert
// could not be delivered, but it never blocks or changes the test result.
type Notifier interface {
	Notify(ctx context.Context, cfg domain.OnFailureConfig, result *domain.TestResult) error
}
