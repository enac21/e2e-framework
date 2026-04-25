package webhook

import (
	"net/http"

	"e2e-framework/internal/core/domain"
)

type Extractor interface {
	Extract(req *http.Request) (*domain.Message, error)
}
