package ports

import (
	"context"

	"e2e-framework/internal/core/domain"
)

type Notifier interface {
	Notify(ctx context.Context, cfg domain.OnFailureConfig, result *domain.TestResult) error
}
