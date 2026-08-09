package ports

import (
	"context"

	"e2e-framework/internal/core/domain"
)

// Notifier executes the on_failure.calls configured for a failed test.
// Implementations are fire-and-forget: a non-nil error means the alert
// could not be delivered, but it never blocks or changes the test result.
type Notifier interface {
	Notify(ctx context.Context, cfg domain.OnFailureConfig, result *domain.TestResult) error
}
