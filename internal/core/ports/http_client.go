package ports

import "net/http"

//go:generate go run -mod=mod go.uber.org/mock/mockgen -source=http_client.go -destination=mocks/mock_http_client.go -package=mocks

type HttpClient interface {
	Do(req *http.Request) (*http.Response, error)
}
