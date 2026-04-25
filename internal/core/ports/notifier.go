package ports

import (
	"context"

	"e2e-framework/internal/core/domain"
)

type Notifier interface {
	Notify(ctx context.Context, result *domain.TestResult) error
}
