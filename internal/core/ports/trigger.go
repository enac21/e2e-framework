package ports

import (
	"context"

	"e2e-framework/internal/core/domain"
)

type Trigger interface {
	Execute(ctx context.Context, def domain.TriggerConfig, runID string, vars map[string]string) (map[string]string, error)
}
