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
| `GET` | `/health` | Liveness check |
| `POST` | `/run-test?id={test_id}` | Trigger a specific test |
| `GET` | `/results` | Last N test results |

---

## Adding a New Test

Create a YAML file in the `tests/` directory. No code changes required:

```yaml
version: "1"
id: my_test
description: "Description of what this test verifies"

schedule: "*/5 * * * *"
enabled: true

trigger:
  method: POST
  url: "https://api.example.com/endpoint"
  timeout: 10s
  headers:
    Content-Type: application/json
  body:
    message_id: "{{run_id}}"

receivers:
  - type: email
    timeout: 30s
    assertions:
      - type: contains
        field: subject
        value: "Expected subject"
```

See `tests/example_welcome_email.yaml` for a complete example.

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

## Roadmap / Pending Tasks

- [ ] **Trigger Data Extraction**: Add the ability to parse the response body of the initial HTTP Trigger to save/extract variables (e.g., a `transaction_id` returned by an API) that can be used later in receiver assertions.
- [ ] **Async API Execution**: The `/run` API endpoint currently blocks until the test finishes. Implement an async mode (returning a tracking ID immediately) and make this behavior configurable per test.
- [ ] **Redis Data Cleanup**: Evaluate whether to explicitly delete test data and reservations from Redis immediately after a test finishes, instead of strictly relying on the key TTL expiration.

---

## Historial de Cambios

- **[2026-04-26]:** Step 13 — Debugging & Refinement. Added initialization logs to HTTP and Webhook servers. Improved `HTTPTrigger` to dynamically support `application/x-www-form-urlencoded` and `application/json` payloads based on headers. Updated `TwilioExtractor` to dynamically extract messages from both JSON and URL-encoded forms depending on the Content-Type. Fixed `run_id` extraction logic.
- **[2026-04-25]:** Step 12 — Main Wiring. Implemented `cmd/server/main.go` wiring the Hexagonal architecture. Started Webhook, API, and Cron servers concurrently with `errgroup` and handled graceful shutdown.
- **[2026-04-25]:** Step 11 — Config & YAML Loader. Implementado parser de configuración y de definiciones de tests usando `gopkg.in/yaml.v3`, con resolución automática de variables de entorno (`{{env.VAR_NAME}}`).
- **[2026-04-25]:** Step 10 — Cron Scheduler. Implementado `Scheduler` usando `robfig/cron/v3` para desencadenar la ejecución automática de los tests definidos en YAML según su cronograma.
- **[2026-04-25]:** Step 9 — HTTP API Server. Implementado servidor HTTP primario para desencadenar tests manualmente (`/run`), chequear salud (`/health`) y ver resultados (`/results`).
- **[2026-04-25]:** Step 8 — Webhook Server. Implementado servidor HTTP primario para recibir webhooks entrantes (Twilio, Meta) y depositarlos en el Redis `Store`.
- **[2026-04-25]:** Step 7 — Orchestrator. Implementado `Orchestrator` que une todos los puertos. Maneja ciclo de vida del test, ejecución concurrente de receivers con `sync.WaitGroup`, agregación de resultados y llamadas al Notifier.
- **[2026-04-25]:** Step 6 — Receiver Adapters. Implementado `ReceiverRegistry` usando patrón factory y 4 receivers (`webhook`, `sms`, `push`, `email`) que hacen polling del `Store` usando un Ticker.
- **[2026-04-25]:** Step 5 — Trigger & Notifier Adapters. Implementados `HTTPTrigger` y `WebhookNotifier` usando un helper nuevo `pkg/template` para el reemplazo recursivo de variables (`{{run_id}}`, etc.).
- **[2026-04-25]:** Step 4 — Assertion Adapters. Implementados 5 assertions (`contains`, `equals`, `matches`, `present`, `not_contains`) y `AssertionRegistry` con patrón factory.
- **[2026-04-25]:** Step 3 — Store Adapter (Redis). Implementado `RedisStore` con `Deposit`, `Claim`, `Reserve` (SetNX atómico), `Release`. Dependencia `go-redis/v9`.
- **[2026-04-25]:** Step 2 — Core Domain. Implemented all domain types (`Message`, `TestResult`, `RunStatus`, `TestDefinition` and sub-types) and all five port interfaces (`Trigger`, `Receiver`, `Assertion`, `Store`, `Notifier`) with full godoc comments. Zero adapter imports in `core/`.
- **[2026-04-25]:** Step 1 — Project Skeleton. Created full directory structure following hexagonal architecture, all placeholder Go files with package declarations and responsibility comments, `Makefile`, `Dockerfile`, `docker-compose.yml`, `configs/config.yaml`, `tests/example_welcome_email.yaml`, `CONTRIBUTING.md`, `CHANGES.md`, and `README.md`.
