package ports

import (
	"context"

	"e2e-framework/internal/core/domain"
)

//go:generate go run -mod=mod go.uber.org/mock/mockgen -source=trigger.go -destination=mocks/mock_trigger.go -package=mocks

type Trigger interface {
	Execute(ctx context.Context, def domain.TriggerConfig, runID string, vars map[string]string) (map[string]string, error)
}
