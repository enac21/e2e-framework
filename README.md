# e2e-testing-service

A data-driven end-to-end testing framework written in Go. Teams define tests via YAML
configuration files — no code changes required. Tests trigger HTTP requests and validate
that the expected notifications arrive through one or more channels (email, SMS, push,
webhook, etc.).

---

## Architecture

The project follows a **hexagonal architecture** (ports and adapters) pattern:

```
adapters/primary   →   core/services   →   ports   ←   adapters/secondary
```

- **`core/`** — Business logic and domain models. Zero external dependencies.
- **`core/ports/`** — Interfaces that define contracts between the domain and the outside world.
- **`adapters/primary/`** — Drive the domain (HTTP API, webhook server, cron scheduler).
- **`adapters/secondary/`** — Driven by the domain (trigger, receivers, store, notifier).
- **`cmd/server/main.go`** — Wiring only. All dependency injection happens here.

### Key Concepts

| Concept | Description |
|---------|-------------|
| **Trigger** | Executes the initial HTTP call that starts the notification flow |
| **Receiver** | Waits for and collects feedback from a notification channel |
| **Assertion** | Validates a field of a NormalizedMessage against an expected value |
| **Store** | Redis-backed temporary buffer with TTL for received messages |
| **Orchestrator** | Coordinates the full test lifecycle, only knows ports |
| **Notifier** | Sends alerts to a configured webhook when a test fails |

---

## Project Structure

```
e2e-testing-service/
├── cmd/server/main.go              # Wiring only
├── internal/
│   ├── core/
│   │   ├── domain/                 # Business models
│   │   ├── ports/                  # Interface definitions
│   │   └── services/               # Orchestrator
│   └── adapters/
│       ├── primary/                # HTTP API, webhook server, cron
│       └── secondary/              # Trigger, receivers, assertions, store, notifier
├── tests/                          # YAML test definitions
├── configs/config.yaml             # Global configuration
├── docker-compose.yml              # Redis + service
├── Dockerfile                      # Multi-stage build
└── Makefile                        # Build/test/deploy targets
```

---

## Prerequisites

- **Go 1.23+**
- **Docker & Docker Compose** (for Redis and containerized deployment)

---

## Quick Start

### Local Development

```bash
# Build the service
make build

# Run unit tests
make test

# Run integration tests (requires Redis)
make test-integration

# Lint the code
make lint
```

### Docker

```bash
# Start the service and Redis
make docker-up

# Stop all services
make docker-down
```

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET`  | `/health` | Liveness check |
| `POST` | `/run?id={test_id}` | Trigger a specific test (sync or async depending on YAML) |
| `GET`  | `/results` | All stored test results (last 100) |
| `GET`  | `/results/{run_id}` | Result for a specific run (supports polling for async) |
| `GET`  | `/swagger/` | Interactive API documentation (Swagger UI) |

---

## Swagger Documentation

To generate or update the API documentation, ensure you have `swag` installed:
```bash
go install github.com/swaggo/swag/cmd/swag@latest
```

Then run the following command from the root directory:
```bash
swag init -g cmd/server/main.go
```

The documentation will be available at `/swagger/index.html` when the service is running.

---

## Adding a New Test

Create a YAML file in the `tests/` directory. No code changes required:

```yaml
version: "1"
id: my_test
description: "Description of what this test verifies"

schedule: "*/5 * * * *"
enabled: true
async: false

trigger:
  method: POST
  url: "https://api.example.com/endpoint"
  timeout: 10s
  headers:
    Content-Type: application/json
  body:
    message_id: "{{run_id}}"
  extract:
    transaction_id: "data.id"

receivers:
  - type: imap
    timeout: 60s
    options:
      host: imap.gmail.com
      port: "993"
      username: test@gmail.com
      password: secret
      mailbox: INBOX
      tls: true
    assertions:
      - type: contains
        field: subject
        value: "Welcome"
  - type: request
    timeout: 60s
    assertions:
      - type: contains
        field: subject
        value: "Welcome"

on_failure:
  webhook:
    url: "https://hooks.slack.com/services/XXX"
    method: POST
    body:
      text: "🚨 Test {{test_id}} failed: {{error}}"
```

### Dynamic Variables

You can dynamically inject values across your test definition using the `{{variable_name}}` syntax:
- `{{run_id}}`: Injected automatically by the Orchestrator. It's the unique UUID for the current test run.
- **Trigger Extraction**: If your trigger hits an API that returns JSON, you can use the `extract` block to map JSON paths (using dot-notation, like `data.id`) to variable names (like `transaction_id`). You can then use these variables in your assertions (e.g., `value: "{{transaction_id}}"`) to validate dynamic runtime data.

See `tests/example_welcome_email.yaml` for a complete example.

### Receiver Options

Some receivers (like `imap`) require connection-specific configuration that can vary per test. Use the `options` block inside the receiver definition to pass any key-value configuration. These options are passed directly to the receiver factory, so each test can target a different server:

```yaml
receivers:
  - type: imap
    timeout: 60s
    options:
      host: imap.company.com
      port: "993"
      username: qa@company.com
      password: secret
      mailbox: INBOX
      tls: "true"
```

For webhook-based receivers (e.g., `request`), the `options` field is not required as those receivers are configured globally in `config.yaml`.

### Retry Logic

By default, a test runs once and is marked as failed if any receiver times out or any assertion does not pass. For flaky or eventually-consistent systems, you can configure automatic retries using the `retry` block:

```yaml
retry:
  enabled: true
  attempts: 3
  delay: 5s
```

- `attempts` — total number of executions (initial + retries). `attempts: 3` means the framework will try up to 3 times before giving up.
- `delay` — how long to wait between attempts. Use standard Go duration strings (`5s`, `1m`, `500ms`).

On each attempt the orchestrator re-creates the receivers, re-fires the trigger and re-collects. If any attempt passes completely, the test is marked as `passed` and no further attempts are made. The `on_failure` webhook (if configured) is only called **once**, after all attempts are exhausted.

> **Note:** Configuration errors (e.g., an unknown receiver `type`) abort immediately and are never retried, since they will not resolve on their own.

---

## Adding a New Receiver

See [CONTRIBUTING.md](CONTRIBUTING.md) for a 5-step guide.

---

## Configuration

Global configuration lives in `configs/config.yaml`. Secrets are injected via
environment variables using the `{{env.VAR_NAME}}` syntax.

Required environment variables:

| Variable | Description |
|----------|-------------|
| `REDIS_URL` | Redis connection URL |
| `WEBHOOK_BASE_URL` | Base URL for the webhook receiver |
| `API_TOKEN` | API token for trigger requests (test-specific) |

---

## Roadmap

The items below are ordered by priority. Completed items are marked ✅.

### ✅ 1. Trigger Variable Injection
Extracted values from the `extract` block are injected into assertion `field` and `value` using `{{variable_name}}` syntax via `template.ReplaceString`. `TestResult.TriggerVars` exposes the resolved values.

### ✅ 2. Retry Logic
The orchestrator reads `retry.enabled`, `retry.attempts` and `retry.delay` from the test YAML. Recipients are reserved once before the retry loop. Receivers are re-created on each attempt. `on_failure` is notified only after all attempts are exhausted. `TestResult.Attempts` records how many tries were made.

### ✅ 3. Security — JWT Authentication
A shared `auth.jwt_secret` (env var `JWT_SECRET`) is used to sign and validate JWTs. The HTTP API validates `Authorization: Bearer <token>`. The Webhook server validates `?token=<jwt>` in the URL (compatible with Twilio, Meta, and any provider that lets you configure the callback URL freely). Both servers are fully bypass-able by setting `auth.enabled: false` for local development.

### 4. IMAP Receiver Implementation
The `IMAPReceiver` skeleton and `ports.IMAPClient` interface already exist. The remaining work is implementing `internal/adapters/secondary/imap_client/client.go` using `github.com/emersion/go-imap/v2`, wiring `Connect`, `SearchByRunID` and `Disconnect`, and removing the `TODO` blanks in the receiver.

### 5. Hexagonal Architecture — IngestUseCase Port (Tech Debt)
The `WebhookServer` currently calls `store.Deposit` directly, bypassing the domain layer. A `ports.MessageIngestor` interface and `services.Ingestor` use case should be introduced so all ingestion logic (validation, enrichment, routing) has a single place.

### 6. Structured Observability
Replace `log.Printf` with `log/slog` (Go 1.21+). Every orchestrator log line should include the `RunID` as a structured field (correlation ID). Optionally expose a `GET /metrics` endpoint for Prometheus.

### 7. Dynamic Hot-Reload
Test YAML files are loaded once at startup. Use `fsnotify` to reload `tests/*.yaml` on change (local mode) or expose a `POST /system/reload` endpoint for CI/CD and Git webhook integration.

### 8. Result Persistence
Replace the in-memory `map[string]*domain.TestResult` (max 100 entries, lost on restart) with a durable store. Proposed: Redis with a JSON blob per `run_id` plus a `ZSET` for chronological listing, and a configurable TTL.

### 9. Improve API JSON Response Messages
Standardise all error responses to return `Content-Type: application/json` with a consistent body:
```json
{ "code": 401, "message": "unauthorized" }
```
Currently `http.Error` returns `text/plain`, which is inconsistent with the JSON success responses.

---

## Changelog

See [CHANGES.md](CHANGES.md) for the full history of changes.
