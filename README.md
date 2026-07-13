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
├── tests/                          # YAML test definitions (subdirectories supported)
├── configs/config.yaml             # Global configuration
├── docker-compose.yml              # Redis + service
├── Dockerfile                      # Multi-stage build
└── Makefile                        # Build/test/deploy targets
```

---

## Prerequisites

- **Go 1.25**
- **Docker & Docker Compose** (for Redis and containerized deployment)

---

## Getting Started

Follow these steps to go from clone to a running end-to-end test:

### 1. Clone and install dependencies

```bash
git clone https://github.com/your-org/e2e-framework.git
cd e2e-framework
go mod download
```

### 2. Create environment variables

The service resolves `{{env.VAR_NAME}}` placeholders in `configs/config.yaml`
and test YAML files using OS environment variables at startup.

Create a `.env` file in the project root (already in `.gitignore`):

```dotenv
# Redis (required)
REDIS_URL=redis://localhost:6379

# JWT authentication secret
# (required even when auth.enabled: false, because the config parser
# resolves {{env.JWT_SECRET}} at load time)
JWT_SECRET=dev-secret-change-me

# IMAP credentials (only needed for tests using the imap receiver)
IMAP_HOST=imap.gmail.com
IMAP_PORT=993
IMAP_USERNAME=you@gmail.com
IMAP_PASSWORD=your-app-password

# Webhook base URL (only needed for webhook-based receiver tests)
WEBHOOK_BASE_URL=http://localhost:8081
```

> **Tip:** Export the variables in your shell (`source .env` won't work on
> most shells — use `export $(grep -v '^#' .env | xargs)` or load them via
> your IDE).

### 3. Start Redis

The service requires a running Redis instance. If you don't have Redis
installed locally, use Docker:

```bash
docker run -d --name e2e-redis -p 6379:6379 redis:7-alpine
```

Alternatively, `make docker-up` starts both Redis **and** the service in
containers (see [Docker](#docker) below).

### 4. Start the server

```bash
make run
```

This compiles the binary (`bin/e2e-testing-service`) and starts it. The API
server listens on **port 8082** and the webhook ingestion server on
**port 8081** (configurable in `configs/config.yaml`).

### 5. Verify the server is running

```bash
curl http://localhost:8082/health
# → {"status":"ok"}
```

`/health` is the only endpoint that doesn't require authentication.

### 6. Run a test

The simplest self-contained test is `local_loop_test`. It triggers the
project's own webhook server and verifies the `request` receiver picks up
the message — no external services needed beyond Redis:

```bash
curl -X POST "http://localhost:8082/run?id=local_loop_test"
```

### 7. Check the result

```bash
curl http://localhost:8082/results
```

You'll see a JSON array with the test result including `status`, `run_id`,
`attempts`, and per-receiver outcomes.

---

### Quick Reference

```bash
# Unit tests
make test

# Integration tests (requires Redis running locally)
make test-integration

# Lint
make lint

# Start everything in Docker (Redis + service)
make docker-up

# Stop Docker services
make docker-down
```

### API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET`  | `/health` | No | Liveness check |
| `POST` | `/run?id={test_id}` | Yes | Trigger a specific test |
| `GET`  | `/results` | Yes | All stored test results (last 100) |
| `GET`  | `/results/{run_id}` | Yes | Result for a specific run (poll for async) |
| `GET`  | `/swagger/` | Yes | Interactive API docs (Swagger UI) |

### Authentication

All endpoints except `/health` require a JWT in the `Authorization` header
when `auth.enabled: true` (the default):

```bash
curl -H "Authorization: Bearer any-non-empty-token" \
     -X POST "http://localhost:8082/run?id=local_loop_test"
```

> **Current behavior:** The middleware only checks that the `Bearer` token is
> present and non-empty. It does **not** validate the token's signature,
> expiration, or claims yet. Any non-empty string works as a token. Full JWT
> validation is planned (see Roadmap item #3).

**To disable authentication for local development**, set this in
`configs/config.yaml`:

```yaml
auth:
  enabled: false
```

Then you can call the API without any `Authorization` header. The `/health`
endpoint never requires auth regardless of this setting.

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

## Development Workflow

### When do I need `make mocks`?

The mock files in `internal/core/ports/mocks/` are already committed to the
repo. You do **not** need to regenerate them for normal development.

Regenerate mocks only when you **add or modify interfaces** in
`internal/core/ports/`:

```bash
# Requires mockgen (already in go.mod as go.uber.org/mock)
make mocks
```

> **Note:** The current `make mocks` target runs `go generate` but the port
> files don't have `//go:generate` directives yet, so the command is a no-op
> in the current state. To make it work, add `//go:generate` lines to each
> port file, or run `mockgen` directly as documented in each mock file's
> header comment.

### When do I need `swag init`?

Swagger docs in `docs/` are already committed. Regenerate them only when you
**add or modify HTTP endpoint annotations** (the `// @Router` comments in
`internal/adapters/primary/http/server.go`):

```bash
swag init -g cmd/server/main.go
```

This updates `docs/docs.go`, `docs/swagger.json`, and `docs/swagger.yaml`.

### `make build` vs `make docker-up`

| | `make build` + `make run` | `make docker-up` |
|---|---|---|
| **What runs** | Native binary on your machine | Docker containers (Alpine) |
| **Redis** | You must start it yourself | Included in compose |
| **Speed** | Fast startup, fast rebuild | Slower (image build + container boot) |
| **Best for** | Day-to-day development, debugging | Verifying production-like behavior, CI |
| **Env vars** | Load from `.env` / shell | Set in `docker-compose.yml` |

For daily development: `make run` + Redis via Docker. For verifying the
container build works: `make docker-up`.

> **Note:** The `Dockerfile` has `EXPOSE 8080` which is inconsistent with the
> actual default port `8082` in `configs/config.yaml`. The `docker-compose.yml`
> correctly maps `8082:8082`.

---

## Adding a New Test

Create a YAML file in `tests/` or any subdirectory. No code changes required:

```yaml
version: "1"
id: my_test
description: "Description of what this test verifies"

schedule: "*/5 * * * *"
enabled: true
async: false

triggers:
  - method: POST
    url: "https://api.example.com/endpoint"
    timeout: 10s
    expected_status: 201
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
    wait_for_receivers: true
  
  - method: GET
    url: "http://localhost:8080/notifications/{{notification_id}}"
    timeout: 10s
    extract:
      status: "status"
    receivers:
      - type: request
        timeout: 15s
        assertions:
          - type: equals
            field: status
            value: "delivered"
    wait_for_receivers: true

on_failure:
  webhook:
    url: "https://hooks.slack.com/services/XXX"
    method: POST
    body:
      text: "Test {{test_id}} failed: {{error}}"
```

> For tests that need multiple HTTP calls in order (e.g., create then verify), add more items to the `triggers` list. Each trigger can have its own receivers and a `wait_for_receivers` flag

### Dynamic Variables

You can dynamically inject values across your test definition using the `{{variable_name}}` syntax:
- `{{run_id}}`: Injected automatically by the Orchestrator. It's the unique UUID for the current test run.
- **Trigger Extraction**: If your trigger hits an API that returns JSON, you can use the `extract` block to map JSON paths (using dot-notation, like `data.id`) to variable names (like `transaction_id`). You can then use these variables in your assertions (e.g., `value: "{{transaction_id}}"`) to validate dynamic runtime data.

See `tests/example_welcome_email.yaml` for a complete example.

### Extract Variables

The `extract` block inside a trigger lets you capture values from the HTTP response body and store them as variables for use in subsequent triggers. The syntax is a map where:

- **Key** = custom variable name (what you choose)
- **Value** = JSON path to extract (dot-notation supported)

```yaml
triggers:
  - method: POST
    url: "https://api.example.com/users"
    body:
      name: "Alice"
    extract:
      user_id: "id"
      full_name: "name"
      org_slug: "organization.slug"
```

This creates variables `{{user_id}}`, `{{full_name}}`, and `{{org_slug}}` that can be used in later triggers:

```yaml
  - method: GET
    url: "https://api.example.com/organizations/{{org_slug}}/members/{{user_id}}"
```

**Rules:**
- Variable names are free-form — use descriptive names like `product_id`, `transaction_id`, etc.
- JSON paths are case-insensitive
- If the path doesn't exist in the response, the variable is silently omitted
- All extracted variables accumulate across triggers — earlier extractions are available in later ones

### Status Code Assertions

By default, any HTTP 4xx or 5xx response from a trigger causes the test step to fail immediately. Use `expected_status` to explicitly assert that a specific status code is returned — this is required for error-path tests where a 4xx response is the correct outcome.

```yaml
triggers:
  # Happy path: assert a resource was created with 201
  - method: POST
    url: "https://api.example.com/users"
    expected_status: 201
    headers:
      Content-Type: application/json
    body:
      name: "Alice"
    extract:
      user_id: "id"

  # Error path: missing required field must return 400
  - method: POST
    url: "https://api.example.com/users"
    expected_status: 400
    headers:
      Content-Type: application/json
    body:
      # name field intentionally omitted

  # Error path: wrong user accessing a resource must return 404
  - method: GET
    url: "https://api.example.com/users/{{user_id}}"
    expected_status: 404
    headers:
      Authorization: "Bearer {{env.OTHER_USER_TOKEN}}"
```

**Rules:**
- When `expected_status` is set, the step passes **only** if the response status matches exactly. Any other code — including 2xx — fails the step.
- When `expected_status` is omitted (default `0`), the original behavior applies: any 4xx/5xx fails the step, any 2xx/3xx passes.
- `extract` still works when `expected_status` matches a 4xx/5xx — the response body is parsed as JSON and fields can be captured (useful for inspecting error payloads).

### Response Assertions

Use `response_assertions` inside a trigger to validate fields in the HTTP response body directly — no receiver needed. This is useful for verifying that a creation returned a valid ID, a GET returned expected data, or an error response has a specific code.

**Assertion types:**

| Type | `field` syntax | Passes when |
|------|---------------|-------------|
| `equals` | dot-path | value is exactly `value` |
| `contains` | dot-path | value contains `value` as a substring |
| `not_contains` | dot-path | value does not contain `value` |
| `present` | dot-path | field exists and is non-empty |
| `matches` | dot-path | value matches the `value` regex pattern |
| `array_contains` | `array[].nested.path` | any element in the array has the nested field equal to `value` |
| `length` | dot-path to array | array has exactly `value` elements |

```yaml
triggers:
  - method: POST
    url: "https://api.example.com/orders"
    headers:
      Content-Type: application/json
    body:
      product: "widget"
      quantity: 3
    extract:
      order_id: "id"
    response_assertions:
      - type: present
        field: "id"
      - type: equals
        field: "status"
        value: "pending"
      - type: contains
        field: "product"
        value: "widget"
      - type: matches
        field: "created_at"
        value: "^\d{4}-\d{2}-\d{2}"
      - type: not_contains
        field: "error"
        value: "failed"

  # Subsequent trigger can use the extracted order_id
  - method: GET
    url: "https://api.example.com/orders/{{order_id}}"
    response_assertions:
      - type: equals
        field: "id"
        value: "{{order_id}}"
      - type: equals
        field: "status"
        value: "pending"
      - type: array_contains
        field: "orders[].address.city"
        value: "Madrid"
      - type: length
        field: "notifications"
        value: "2"
```

### Step Delay

Use `delay_before` on any trigger step to pause execution for a fixed duration before that step fires. Useful when an upstream service processes events asynchronously and the verify step needs to wait for propagation.

```yaml
triggers:
  # Step 1: Create resource via async service
  - method: POST
    url: "{{env.GCM_BASE_URL}}/v1/inbox/notifications"
    headers:
      Authorization: "Bearer {{env.GCM_TOKEN}}"
    body:
      user_id: "abc-123"
    extract:
      notification_id: "id"

  # Step 2: Wait 3s for async processing, then verify
  - method: GET
    url: "{{env.INBOX_BASE_URL}}/v1/abc-123/notifications/{{notification_id}}"
    delay_before: 3s
    expected_status: 200
    headers:
      Authorization: "Bearer {{env.INBOX_TOKEN}}"
```

**Rules:**
- `delay_before` accepts any Go duration string: `500ms`, `2s`, `1m`, etc.
- The delay runs **once per step** — before the first attempt. Retries do not repeat the delay (they use `retry.delay` instead).
- Omitting `delay_before` (or setting it to `0`) skips the delay entirely.
- The delay is logged: `[run-id] step N waiting Xs before execution`.

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
environment variables using the `{{env.VAR_NAME}}` syntax. The config loader
resolves these placeholders **before** YAML parsing, so the parser always sees
plain values.

### Full config reference

```yaml
# configs/config.yaml

version: "1"

server:
  port: 8082           # HTTP API port
  timeout: 30s

auth:
  enabled: true        # Set to false for local development to skip JWT
  jwt_secret: "{{env.JWT_SECRET}}"

webhook:
  port: 8081           # Webhook ingestion server port (Twilio, Meta, etc.)

store:
  redis:
    url: "{{env.REDIS_URL}}"
    ttl: 300s          # How long received messages are kept

scheduler:
  enabled: true
  timezone: "Europe/Madrid"

tests:
  path: "./tests"      # Directory containing YAML test definitions

receivers:
  sms:
    provider: twilio
    # fields TBD during SmsReceiver implementation
  push:
    provider: fcm
    # fields TBD during PushReceiver implementation
  webhook:
    base_url: "{{env.WEBHOOK_BASE_URL}}"

logging:
  level: info           # debug | info | warn | error
  format: json          # json (production) | text (local)
```

### Environment variables

| Variable | Required | Used in | Description |
|----------|----------|---------|-------------|
| `REDIS_URL` | Yes | `config.yaml` | Redis connection URL (e.g. `redis://localhost:6379`) |
| `JWT_SECRET` | Yes | `config.yaml` | Shared secret for JWT signing/validation |
| `WEBHOOK_BASE_URL` | No | `config.yaml` | Base URL for webhook receiver callbacks |
| `IMAP_HOST` | No | Test YAMLs | IMAP server hostname for email tests |
| `IMAP_PORT` | No | Test YAMLs | IMAP server port (typically `993`) |
| `IMAP_USERNAME` | No | Test YAMLs | IMAP login username/email |
| `IMAP_PASSWORD` | No | Test YAMLs | IMAP login password or app password |
| `API_TOKEN` | No | Test YAMLs | Bearer token for APIs called by test triggers |

> **Note:** The `{{env.*}}` syntax works in both `configs/config.yaml` and
> individual test YAML files in `tests/`. The same resolver handles both.

---

## Roadmap

The items below are ordered by priority. Completed items are marked ✅.

### ✅ 1. Trigger Variable Injection
Extracted values from the `extract` block are injected into assertion `field` and `value` using `{{variable_name}}` syntax via `template.ReplaceString`. `TestResult.TriggerVars` exposes the resolved values.

### ✅ 2. Retry Logic
The orchestrator reads `retry.enabled`, `retry.attempts` and `retry.delay` from the test YAML. Recipients are reserved once before the retry loop. Receivers are re-created on each attempt. `on_failure` is notified only after all attempts are exhausted. `TestResult.Attempts` records how many tries were made.

### ✅ 3. Security — JWT Authentication
A shared `auth.jwt_secret` (env var `JWT_SECRET`) is used to sign and validate JWTs. The HTTP API validates `Authorization: Bearer <token>`. The Webhook server validates `?token=<jwt>` in the URL (compatible with Twilio, Meta, and any provider that lets you configure the callback URL freely). Both servers are fully bypass-able by setting `auth.enabled: false` for local development.

### ✅ 4. IMAP Receiver Implementation
The `IMAPReceiver` skeleton and `ports.IMAPClient` interface already exist. The remaining work is implementing `internal/adapters/secondary/imap_client/client.go` using `github.com/emersion/go-imap/v2`, wiring `Connect`, `SearchByRunID` and `Disconnect`, and removing the `TODO` blanks in the receiver.

### ✅ 5. Multiple & Sequential Triggers
Tests can now define multiple triggers in order using the `triggers` key. Each trigger groups an HTTP call with its own receivers and a `wait_for_receivers` flag. Variables extracted in earlier triggers accumulate and are available in later triggers.

### ✅ 6. Trigger Response Assertions
Each trigger now supports two assertion mechanisms that run directly against the HTTP response — no receiver required:

- **`expected_status`** — asserts the response status code matches exactly. Enables error-path testing where a 4xx/5xx is the correct outcome. See [Status Code Assertions](#status-code-assertions).
- **`response_assertions`** — asserts fields in the JSON response body using the same assertion types as receivers (`equals`, `contains`, `not_contains`, `present`, `matches`). Field paths use dot-notation and are case-insensitive. Values support `{{variable}}` substitution. See [Response Assertions](#response-assertions).

### 7. Hexagonal Architecture — IngestUseCase Port (Tech Debt)
The `WebhookServer` currently calls `store.Deposit` directly, bypassing the domain layer. A `ports.MessageIngestor` interface and `services.Ingestor` use case should be introduced so all ingestion logic (validation, enrichment, routing) has a single place.

### 8. Dynamic Hot-Reload
Test YAML files are loaded once at startup. Use `fsnotify` to reload `tests/*.yaml` on change (local mode) or expose a `POST /system/reload` endpoint for CI/CD and Git webhook integration.

### 9. Result Persistence
Replace the in-memory `map[string]*domain.TestResult` (max 100 entries, lost on restart) with a durable store. Proposed: Redis with a JSON blob per `run_id` plus a `ZSET` for chronological listing, and a configurable TTL.

### 10. Improve API JSON Response Messages
Standardise all error responses to return `Content-Type: application/json` with a consistent body:
```json
{ "code": 401, "message": "unauthorized" }
```
Currently `http.Error` returns `text/plain`, which is inconsistent with the JSON success responses.

### 11. Production-Ready Console Logging System
Implement a reworked, structured logging system (e.g., using `log/slog`) that outputs strictly to standard console (stdout/stderr). This ensures logs are properly captured, parseable, and fully functional when the project is deployed in production environments like containers, Kubernetes, or any other cloud-native orchestrator.

### 12. Comprehensive Documentation & YAML Reference
Review and enhance the `README.md` documentation. The primary goal is to thoroughly document each feature and rule of the framework strictly from the perspective of the YAML configuration file, providing clear examples and use cases for end-users to understand how to leverage all capabilities.

---

## Changelog

See [CHANGES.md](CHANGES.md) for the full history of changes.
