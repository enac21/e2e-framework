# Build stage
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /app

# Copy dependency files first for layer caching
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code (tests are mounted externally in production)
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY docs/ ./docs/
COPY configs/ ./configs/

# Build the binary
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /app/bin/e2e-testing-service ./cmd/server/

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy the binary and default config from builder
COPY --from=builder /app/bin/e2e-testing-service .
COPY --from=builder /app/configs ./configs

EXPOSE 8080

ENTRYPOINT ["./e2e-testing-service"]
