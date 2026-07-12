# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy dependency files first for layer caching
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/e2e-testing-service ./cmd/server/

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/bin/e2e-testing-service .

# Copy config and test files
COPY --from=builder /app/configs ./configs
COPY --from=builder /app/tests ./tests

EXPOSE 8080

ENTRYPOINT ["./e2e-testing-service"]
