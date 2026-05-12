# NOTE: mingw32-make in windows

.PHONY: build run test test-integration lint clean docker-build docker-up docker-down

# Binary output name
BINARY_NAME=e2e-testing-service

# Build the application
build:
	go build -o bin/$(BINARY_NAME) ./cmd/server/

# Run the application locally
run: build
	./bin/$(BINARY_NAME)

# Run unit tests
test:
	go test ./... -v -count=1

# Run integration tests (requires running Redis)
test-integration:
	go test ./... -v -count=1 -tags=integration

# Run linter
lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run

# Generate mocks
mocks:
	go generate ./internal/core/ports/...

# Build Docker image
docker-build:
	docker build -t e2e-testing-service .

# Start all services with Docker Compose
docker-up:
	docker compose up -d

# Stop all services with Docker Compose
docker-down:
	docker compose down
