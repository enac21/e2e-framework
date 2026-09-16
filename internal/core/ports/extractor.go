package ports

import (
	"net/http"

	"e2e-framework/internal/core/domain"
)

//go:generate go run -mod=mod go.uber.org/mock/mockgen -source=extractor.go -destination=mocks/mock_extractor.go -package=mocks

type Extractor interface {
	Extract(req *http.Request) (*domain.Message, error)
}
