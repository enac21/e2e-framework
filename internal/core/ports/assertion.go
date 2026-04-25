package ports

import "e2e-framework/internal/core/domain"

type Assertion interface {
	Assert(msg *domain.Message) error
}
