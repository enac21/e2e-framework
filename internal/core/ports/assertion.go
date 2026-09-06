package ports

import "e2e-framework/internal/core/domain"

//go:generate go run -mod=mod go.uber.org/mock/mockgen -source=assertion.go -destination=mocks/mock_assertion.go -package=mocks

type Assertion interface {
	Assert(msg *domain.Message) error
}
