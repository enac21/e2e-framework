# e2e-testing-service

A data-driven end-to-end testing framework written in Go. Define tests via YAML
configuration files — no code changes required. Tests trigger HTTP requests and validate
the responses throw trigger response asserts and receivers.

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
| **Notifier** | Executes the `on_failure.calls` alerts when a test fails |

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
- **Docker & Docker Compose** (for dependencies and containerized deployment)

---

## Getting Started

You have **two ways** to run the service:

- **Option A — Published Docker image (recommended)** — pull the versioned container from `ghcr.io` and start it with your own config/tests, no Go toolchain needed.
- **Option B — Build from source** — clone the repo and run natively.

Both use the same config resolution (`{{env.VAR_NAME}}` in YAML + `CONFIG_PATH`).

### Option A — Published Docker image (fastest)

The service is published as a versioned, multi-arch image at `ghcr.io/enac21/e2e-framework` (see `Makefile:docker-build-multiarch`). Use `:latest` for development or pin to a version tag (e.g. `:v1.2.0`) in production.

#### 1. Pull the image

```bash
docker pull ghcr.io/enac21/e2e-framework:latest
# or a pinned version
docker pull ghcr.io/enac21/e2e-framework:v1.2.0
```

#### 2. Create environment variables

The service resolves `{{env.VAR_NAME}}` placeholders in `configs/config.yaml`
and test YAML files at startup (before YAML parsing).

Create a `.env` file (already in `.gitignore`):

```dotenv
# Store backend — only the selected backend needs its URL/DSN
REDIS_URL=redis://redis:6379
# POSTGRES_DSN=postgres://e2e:e2e@postgres:5432/e2e
# STORE_TYPE=memory   # overrides store.type in YAML (redis|postgres|memory|disabled)

# JWT authentication secret
# (required even when auth.enabled: false — the loader resolves {{env.JWT_SECRET}} at startup)
JWT_SECRET=dev-secret-change-me

# Only needed for tests that use the imap / webhook receivers
IMAP_HOST=imap.gmail.com
IMAP_PORT=993
IMAP_USERNAME=you@gmail.com
IMAP_PASSWORD=your-app-password
WEBHOOK_BASE_URL=http://localhost:8081
```

> **Tip:** Export the variables in your shell (`source .env` won't work on
> most shells — use `export $(grep -v '^#' .env | xargs)` or load them via
> your IDE / compose `env_file`).

#### 3. Run with Docker (custom config + custom tests)

The image already contains the default `configs/config.yaml` and `tests/` from the build, but you should **mount your own** to configure the service without rebuilding:

- **Config:** mount a custom YAML and point `CONFIG_PATH` at it (`cmd/server/main.go:39` defaults to `configs/config.yaml`).
- **Tests:** mount a host directory to the path declared in `tests.path` (default `tests`).

```bash
# Single container — API on 8082 (override via server.port in your config)
docker run --rm -p 8082:8082 --env-file .env \
  -v $(pwd)/my-config.yaml:/app/my-config.yaml:ro \
  -v $(pwd)/my-tests:/app/my-tests:ro \
  -e CONFIG_PATH=/app/my-config.yaml \
  ghcr.io/enac21/e2e-framework:latest

# With docker compose — replace build: . with image:
# docker-compose.yml
# services:
#   e2e-service:
#     image: ghcr.io/enac21/e2e-framework:latest   # ← instead of build: .
#     ports: ["8082:8082"]          # must match server.port in your config
#     env_file: .env
#     environment:
#       - CONFIG_PATH=/app/my-config.yaml
#     volumes:
#       - ./my-config.yaml:/app/my-config.yaml:ro
#       - ./my-tests:/app/my-tests:ro
#     depends_on:
#       redis: {condition: service_healthy}

docker compose up -d redis          # start deps
docker compose up -d e2e-service    # start service from published image
```

Minimal custom config example (`my-config.yaml`):

```yaml
version: "1"
server:
  port: 8082
auth:
  enabled: false
  jwt_secret: "{{env.JWT_SECRET}}"
store:
  type: memory        # no external DB for local dev
  memory: {ttl: 300s}
tests:
  path: "/app/my-tests"
```

> **Notes:**
> - The image `EXPOSE`s `8080` (`Dockerfile:33`), but the actual listen port is `server.port` in your config (default `8082` in `configs/config.example.yaml`). Map the host port to match `server.port`.
> - `CONFIG_PATH` and `STORE_TYPE` are the only env vars read directly by the binary; everything else is injected via `{{env.*}}` placeholders in YAML.
> - Tests are loaded once at startup via `tests.path` (`cmd/server/main.go:49`); mount them read-only (`:ro`).

#### 4. Verify, run a test, check results

```bash
curl http://localhost:8082/health
# → {"status":"ok"}   # /health never requires auth

curl -X POST "http://localhost:8082/run?id=local_loop_test"
curl http://localhost:8082/results
```

---

### Option B — Build from source

Follow these steps to go from clone to a running end-to-end test natively.

#### 1. Clone and install dependencies

```bash
git clone https://github.com/your-org/e2e-framework.git
cd e2e-framework
go mod download
```

#### 2. Create environment variables

Same `.env` as in Option A, step A2. The service resolves `{{env.VAR_NAME}}` placeholders in `configs/config.yaml` and test YAML files using OS environment variables at startup.

```dotenv
# Redis (required if store.type: redis)
REDIS_URL=redis://localhost:6379
JWT_SECRET=dev-secret-change-me
IMAP_HOST=imap.gmail.com
IMAP_PORT=993
IMAP_USERNAME=you@gmail.com
IMAP_PASSWORD=your-app-password
WEBHOOK_BASE_URL=http://localhost:8081
```

#### 3. Start dependencies

The service requires a running store for `webhook` receivers (or use `store.type: memory` / `disabled` for DB-less runs):

```bash
docker run -d --name e2e-redis -p 6379:6379 redis:7-alpine
# or for postgres: docker run -d --name e2e-pg -p 5432:5432 -e POSTGRES_USER=e2e -e POSTGRES_PASSWORD=e2e postgres:16-alpine
```

Alternatively, `make docker-up` builds the image locally and starts both Redis **and** the service in containers.

#### 4. Start the server

```bash
make run
# or: CONFIG_PATH=./my-config.yaml go run ./cmd/server
```

This compiles the binary (`bin/e2e-testing-service`) and starts it. The HTTP API + webhook ingestion share `server.port` (configurable in `configs/config.yaml`, default `8082`).

#### 5. Verify the server is running

```bash
curl http://localhost:8082/health
# → {"status":"ok"}
```

`/health` is the only endpoint that doesn't require authentication.

#### 6. Run a test

The simplest self-contained test is `local_loop_test`. It triggers the
project's own webhook server and verifies the `webhook` receiver picks up
the message — no external services needed beyond Redis:

```bash
curl -X POST "http://localhost:8082/run?id=local_loop_test"
```

#### 7. Check the result

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

# Pull and run the published image (no Go needed)
docker pull ghcr.io/enac21/e2e-framework:latest
docker run --rm -p 8082:8082 --env-file .env \
  -v $(pwd)/my-config.yaml:/app/my-config.yaml:ro \
  -v $(pwd)/my-tests:/app/my-tests:ro \
  -e CONFIG_PATH=/app/my-config.yaml \
  ghcr.io/enac21/e2e-framework:latest

# Or build locally and start everything (Redis + service)
make docker-up

# Stop Docker services
make docker-down
```

### API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET`  | `/health` | No | Liveness check |
| `POST` | `/run?id={test_id}` | Yes | Trigger a specific test |
| `POST` | `/run-sequence` | Yes | Run an ordered sequence of tests (see [Run a test group](#run-a-test-group)) |
| `GET`  | `/results` | Yes | All stored test results (last 100) |
| `GET`  | `/results/{run_id}` | Yes | Result for a specific run (poll for async) |
| `GET`  | `/swagger/` | Yes | Interactive API docs (Swagger UI) |

### Run a test group

`POST /run-sequence` runs tests in order. The **body is a plain JSON array of
test IDs**; alternatively, a `test_group` query param runs a named,
pre-configured group:

```bash
# Explicit list of test IDs
curl -X POST http://localhost:8082/run-sequence \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '["crear_y_verificar_producto","local_loop_test"]'

# Named, pre-configured group (test_group as query param)
curl -X POST "http://localhost:8082/run-sequence?test_group=ci" \
  -H "Authorization: Bearer $TOKEN"
```

**CI/CD story:** a group lets you change *which* tests a pipeline runs by
editing `configs/config.yaml` only — the script/call that hits the endpoint
never changes:

```yaml
test_groups:
  ci:
    description: "Pipeline that runs after every merge"
    tests:
      - crear_y_verificar_producto
      - local_loop_test
    test_delay: 2s
    skip_fail_test: true
```

- `tests` — the ordered list of test IDs to run.
- `test_delay` / `skip_fail_test` — optional per-group defaults for the
  `test_delay` and `skip_fail_test` query params.
- `test_group` and a body test list are **mutually exclusive** (400 if both
  are sent).
- An unknown `test_group` returns `404`; an empty list returns `400`.

**Precedence:** explicit query params (`test_delay`, `skip_fail_test`) always
win; otherwise the group's defaults apply; otherwise the built-in defaults
(`0`, `false`).

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

### `make build` vs `make docker-up` vs published image

| | `make build` + `make run` | `make docker-up` (local build) | Published `ghcr.io/enac21/e2e-framework` |
|---|---|---|---|
| **What runs** | Native binary on your machine | Docker containers (Alpine, built locally) | Same container, pulled pre-built (multi-arch) |
| **Redis** | You must start it yourself | Included in compose | Included if you use the compose file with `image: ghcr.io/...` |
| **Speed** | Fast startup, fast rebuild | Slower (image build + container boot) | Fastest — no build, just `docker pull` |
| **Best for** | Day-to-day development, debugging | Verifying the Dockerfile works | Production, CI, onboarding without Go |
| **Env vars** | Load from `.env` / shell | Set in `docker-compose.yml` | `env_file` / `-e CONFIG_PATH` + volume mounts for custom config/tests |
| **Custom config** | `CONFIG_PATH=./my-config.yaml make run` | `volumes: - ./my-config.yaml:/app/my-config.yaml:ro` + `CONFIG_PATH` | Same — `docker run -v $(pwd)/my-config.yaml:/app/my-config.yaml:ro -e CONFIG_PATH=/app/my-config.yaml ghcr.io/enac21/e2e-framework:latest` |

For daily development: `make run` + Redis via Docker. For verifying the
container build works: `make docker-up`. For production/onboarding: `docker pull ghcr.io/enac21/e2e-framework:latest` and mount your config/tests (see [Option A](#option-a--published-docker-image-fastest)).

> **Note:** The `Dockerfile` has `EXPOSE 8080` which is inconsistent with the
> actual default port `8082` in `configs/config.yaml`. The `docker-compose.yml`
> correctly maps `8082:8082`.

---

## Adding a New Test

Create a YAML file in `tests/` or any subdirectory. No code changes required.
For a field-by-field reference with every default annotated see `tests/example_all_fields.yaml` — each README section below also links to its focused example:

```yaml
version: "1"
id: my_test
description: "Description of what this test verifies"

schedule: "*/5 * * * *"
enabled: true
async: false          # true → POST /run returns 202 {run_id, status:"running"} immediately and test continues in background (useful with wait_for_receivers). See [Async Mode](#async-mode-asynctrue)

variables:
  base_url: "{{env.BASE_URL}}"
  request_id: "{{uuid()}}"

triggers:
  - method: POST
    url: "{{base_url}}/endpoint"
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
      - type: webhook
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
      - type: webhook
        timeout: 15s
        assertions:
          - type: equals
            field: status
            value: "delivered"
    wait_for_receivers: true

on_failure:
  calls:
    - method: POST
      url: "https://hooks.slack.com/services/XXX"
      timeout: 10s
      headers:
        Content-Type: application/json
      body:
        text: "Test {{test_id}} failed: {{error}}"
    - method: POST
      url: "https://alerts.company.com/tickets"
      body:
        test_id: "{{test_id}}"
        run_id: "{{run_id}}"
        error: "{{error}}"
```

> For tests that need multiple HTTP calls in order (e.g., create then verify), add more items to the `triggers` list. Each trigger can have its own receivers and a `wait_for_receivers` flag

**`wait_for_receivers` vs `async`:** `wait_for_receivers: true` makes the orchestrator block that step until all its `receivers` complete (`timeout` per receiver, 1s polling for `webhook`) and their `assertions` pass; `false` only fires the `trigger` and moves to the next without waiting. It is independent of `async: true` — with `async: true` `POST /run` returns `202 {run_id, status:"running"}` **immediately** even if the step has `wait_for_receivers: true`; the `wait` still happens in background and you poll `GET /results/{run_id}` until `passed/failed/error`. Ideal for webhooks with long `timeout`.

Each trigger may also declare a `type` (defaults to `http`) and an `options` map. `http` is the only trigger type bundled; its factory receives the shared `response_assertions` registry and currently ignores trigger-level `options` (they are passed to the factory and reserved for future trigger implementations). Triggers are built through a `TriggerRegistry` following the same factory pattern as receivers, so adding a new trigger type means registering a new factory in `main.go` and referencing it per-step (`type: my_type`).

### Dynamic Variables

You can dynamically inject values across your test definition using the `{{variable_name}}` syntax:
- `{{run_id}}`: Injected automatically by the Orchestrator. It's the unique UUID for the current test run.
- **Trigger Extraction**: If your trigger hits an API that returns JSON, you can use the `extract` block to map JSON paths (using dot-notation, like `data.id`) to variable names (like `transaction_id`). You can then use these variables in your assertions (e.g., `value: "{{transaction_id}}"`) to validate dynamic runtime data.

There are three sources of variables, in increasing precedence order (later sources override earlier ones on name collision):

1. **Built-in**: `run_id` and `test_id` (see below).
2. **`variables:` block**: static aliases and one-shot generators (see below).
3. **`extract`**: variables captured from a trigger's JSON response. These overwrite a `{{name}}` from `variables:` if both share the same name.

**Reserved names:** `run_id`, `test_id` and `error` cannot be defined in the `variables:` block. If you try, the value is ignored and a warning is logged.

> **Examples:** `tests/example_welcome_email.yaml` (full flow with `variables:` + `extract`), `tests/example_variables.yaml` (one-shot `{{uuid()}}` stability), `tests/example_all_fields.yaml` (exhaustive reference with every variable source).

### The `variables:` Block

The optional `variables:` block lets you define reusable values at the top of a test. Each value is evaluated **once** when the run starts, then stays **stable for the whole test** — across every trigger, receiver, assertion and `on_failure` call. This is useful for:

- **URLs and hosts that repeat** across many triggers (only need changing in one place).
- **One-shot generators** like `{{uuid()}}` or `{{randomInt(N)}}` that must yield the *same* value everywhere (e.g. an idempotency key sent in the payload and later asserted). Unlike using the generator inline — which produces a *new* value on every occurrence — a variable captures a single value for the whole run.

```yaml
id: my_test
variables:
  base_url: "{{env.BASE_URL}}"
  request_id: "{{uuid()}}"
  otp_code: "{{randomInt(6)}}"

triggers:
  - method: POST
    url: "{{base_url}}/v1/users"
    body:
      request_id: "{{request_id}}"     # same UUID everywhere
      otp_code: "{{otp_code}}"         # same 6-digit code everywhere
  - method: GET
    url: "{{base_url}}/v1/users/{{request_id}}"
    response_assertions:
      - type: contains
        field: request_id
        value: "{{request_id}}"        # still the same UUID

on_failure:
  calls:
    - url: "https://alerts.example.com"
      body:
        request_id: "{{request_id}}"   # available in notifications too
```

**Rules:**
- `variables:` is a simple `name: value` map.
- Values may use other variables (`{{env.X}}`, `{{run_id}}`, previously-defined `variables`, and generators).
- Each variable is resolved once at start-up of the run; generators are not re-evaluated.
- Reserved names (`run_id`, `test_id`, `error`) are ignored with a warning.
- An `extract` value with the same name **overrides** the `variables:` value.

> **Example:** `tests/example_variables.yaml` — stable `request_id: "{{uuid()}}"` reused across triggers; `tests/example_all_fields.yaml` — every `variables:` default annotated.

### Reusing YAML Blocks (Anchors)

The framework parses YAML with [`gopkg.in/yaml.v3`](https://github.com/go-yaml/yaml), which supports **YAML anchors and merge keys natively — no configuration needed**. You can define a reusable block (a full trigger, receiver, set of headers, etc.) once at the top of a test and apply it to many triggers. This is the "variable" equivalent for *whole YAML structures* rather than scalar values.

There are **two ways** to reuse an anchor:

#### 1. Alias (`*anchor`) — replace the whole block

Use `*anchor` when the block is **already complete** and identical everywhere: it substitutes the entire anchored value as-is, with nothing added or overridden.

```yaml
id: my_test
# A fully-specified shared block, defined with an anchor (&ping) under an x- key
# (the x- prefix keeps it out of the framework's schema).
x-ping: &ping
  method: GET
  url: "{{env.BASE_URL}}/health"
  timeout: 5s
  expected_status: 200

triggers:
  - *ping        # reuses the whole block exactly as defined
  - *ping
  - *ping
```

#### 2. Merge key (`<<: *anchor`) — reuse the block and add/override

Use `<<: *anchor` when you want to start from the shared block **and** vary it per trigger: it merges the anchored keys as a base, and any key you write on that trigger **overrides** the anchored value.

```yaml
id: my_test
x-post-json: &post_json
  method: POST
  timeout: 10s
  headers:
    Content-Type: application/json
  wait_for_receivers: true

variables:
  base_url: "{{env.BASE_URL}}"

triggers:
  - <<: *post_json                      # reuse method/timeout/headers
    url: "{{base_url}}/users"
    body: { name: "Alice" }
  - <<: *post_json                      # reuse again, override url/body only
    url: "{{base_url}}/users/{{user_id}}"
    body: { name: "Bob" }
```

**Rules:**
- **Anchor within the same file only**: `&name` / `*name` / `<<: *name` reuse blocks *inside one YAML file*. Cross-file sharing is not supported.
- Use the `x-` prefix (e.g. `x-post-json`) so the anchor block is not interpreted as a framework field.
- `*anchor` is a **full replacement** — you cannot add or override fields on that node.
- `<<: *anchor` **merges** the anchored keys into the current map; anything you write on that map overrides the anchored value.
- Anchors and the `variables:` block are complementary: use `variables:` for values (URLs, IDs, tokens) and anchors for complete structures (headers, receivers, options).

> **Example:** `tests/example_variables.yaml` — `x-post-json: &post_json` reused via `<<: *post_json`; `tests/example_all_fields.yaml` — same anchor plus a full-replacement `*ping` pattern.

### Template Generators

Besides variables, the framework evaluates **generators** — `{{name(args)}}` tags that produce a fresh value every time they are resolved. They work anywhere template resolution happens: URLs, headers, request/response bodies, assertions and `on_failure.calls`.

| Generator | Syntax | Description |
|---|---|---|
| `{{randomInt(N)}}` | `randomInt(6)` | Random integer with at most `N` digits, in the range `[0, 10^N)`. |
| `{{uuid()}}` | `uuid()` | Random UUID (v4) string, e.g. `2f4c3a1e-...-...-...-...`. |

Each occurrence is evaluated independently, so the same tag used twice in one payload yields two different values (useful for idempotency keys that must be unique per field):

```yaml
triggers:
  - method: POST
    url: "https://api.example.com/v1/user"
    body:
      request_id: "{{uuid()}}"        # unique per request
      otp_code: "{{randomInt(6)}}"    # 6-digit code
```

A generator whose arguments are invalid (e.g. `{{randomInt(abc)}}` or `{{uuid(v4)}}`) leaves the placeholder untouched, and the tag is reported by `HasUnresolved` as an unresolved placeholder.

> **Example:** `tests/example_variables.yaml` — `{{uuid()}}`/`{{randomInt(6)}}` as one-shot via `variables:`; `tests/example_all_fields.yaml` — inline per-occurrence vs stable comparison.

### Increment/Decrement Operator

Unlike stateless generators, `{{++(var_name)}}` and `{{--(var_name)}}` are **stateful operators**: they mutate a test variable and persist that mutation. The variable is replaced by the with the new value and the change survives across trigger steps.

Syntactically they are bracketed like generators: `{{++(counter)}}` and `{{--(counter)}}`.

**Two evaluation moments:**

1. **Before the request** — inside a trigger's `url`, `headers` or `body`: operates on variables that already exist (`variables:` block, or extracted by a *previous* trigger). Useful for building sequences of distinct request values.
2. **After the request / in later steps** — once a previous trigger's `extract` has populated a variable, `{{++(var)}}` in a subsequent trigger (request, assertion value, `on_failure.calls`) advances it again.

The operator is **not** used inside the `extract` block itself; it operates on already-extracted variables.

```yaml
variables:
  counter: 0

triggers:
  - method: POST
    url: "{{env.BASE_URL}}/items"
    body:
      seq: "{{++(counter)}}"        # sends 1, persists counter=1
  - method: POST
    url: "{{env.BASE_URL}}/items"
    body:
      seq: "{{++(counter)}}"        # sends 2, persists counter=2
```

**Rules:**
- Replaces the variable with the new value and **persists** it: `{{++(counter)}}` with `counter=3` → `4` and `counter` stays `4`.
- If the variable **does not exist**, it is treated as `0` and created by the operator: `{{++(seq)}}` on an undefined `seq` → `1` (leaving `seq=1`), `{{--(seq)}}` → `-1`.
- If the variable **exists but is not an integer** (e.g. `"abc"`), the variable is left unresolved: the trigger aborts with a clear error, the `on_failure.calls` is skipped, and assertions simply don't match.

> **Example:** `tests/example_increment_int_assertions.yaml` — `{{++(counter)}}`/`{{--(counter)}}` in headers/body/URL (steps 1-3); `tests/example_all_fields.yaml` — counter in URL + body with `{{counter}}` after mutation.

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

> **Example:** `tests/crud_productos.yaml` + `tests/crear_y_verificar_producto.yaml` — `extract: {id: "id"}` then `GET /productos/{{id}}`; `tests/example_all_fields.yaml` — every `extract` path + case-insensitivity annotated.

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

On each attempt the orchestrator re-creates the receivers, re-fires the trigger and re-collects. If any attempt passes completely, the test is marked as `passed` and no further attempts are made. The `on_failure.calls` notifications (if configured) are only executed **once**, after all attempts are exhausted.

> **Note:** Configuration errors (e.g., an unknown receiver `type`) abort immediately and are never retried, since they will not resolve on their own.

> **Example:** `tests/local_loop_test.yaml` — `retry: {enabled:true, attempts:3, delay:5s}`; `tests/crud_productos.yaml` + `tests/crear_y_verificar_producto.yaml` — `attempts:2`; `tests/example_all_fields.yaml` — `retry` defaults annotated.

### Async Mode (`async: true`)

`async` is at test level, `wait_for_receivers` at trigger level — they are orthogonal:

```yaml
async: true          # test does not block POST /run
triggers:
  - method: POST
    url: "{{env.app_base_url}}/orders"
    body: { product_id: "sku-42", message_id: "{{run_id}}" }
    receivers:
      - type: webhook
        timeout: 60s
    wait_for_receivers: true  # still waits for webhook in background
```

* `async: false` (default): `POST /run?id={id}` blocks until all `triggers` (plus their `wait_for_receivers`) finish and returns `200` with the full `TestResult` (`passed/failed/error`).
* `async: true`: `POST /run?id={id}` returns immediately `202 {"run_id":"<uuid>","status":"running"}` and the test continues in background respecting each `wait_for_receivers` and `timeout`. Poll progress with `GET /results/{run_id}` until final state. Essential for webhooks with long `timeout` to avoid holding the HTTP connection open.

`POST /run-sequence` is always synchronous today and returns `200 [TestResult]` at the end; for async flows use `POST /run` per test.

> **Example:** `tests/example_generic_webhook.yaml` — `async: true` with `wait_for_receivers: true`; `tests/manual_imap_test.yaml` — `async: true` IMAP long poll; `tests/example_all_fields.yaml` — `async: false` default annotated.

---

### Step Delay

Use `delay_before` on any trigger step to pause execution for a fixed duration before that step fires. Useful when an upstream service processes events asynchronously and the verify step needs to wait for propagation.

```yaml
triggers:
  # Step 1: Create resource via async service
  - method: POST
    url: "{{env.BASE_URL}}/v1/test/notifications"
    headers:
      Authorization: "Bearer {{env.BASE_TOKEN}}"
    body:
      user_id: "abc-123"
    extract:
      notification_id: "id"

  # Step 2: Wait 3s for async processing, then verify
  - method: GET
    url: "{{env.BASE_URL_2}}/v1/abc-123/notifications/{{notification_id}}"
    delay_before: 3s
    expected_status: 200
    headers:
      Authorization: "Bearer {{env.BASE_2_TOKEN}}"
```

**Rules:**
- `delay_before` accepts any Go duration string: `500ms`, `2s`, `1m`, etc.
- The delay runs **once per step** — before the first attempt. Retries do not repeat the delay (they use `retry.delay` instead).
- Omitting `delay_before` (or setting it to `0`) skips the delay entirely.
- The delay is logged: `[run-id] step N waiting Xs before execution`.

> **Example:** `tests/example_all_fields.yaml` — `delay_before: 3s` on the verification `GET` step (step 2).

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

> **Example:** `tests/example_increment_int_assertions.yaml` — `expected_status: 200` on webhook.cool steps; `tests/example_all_fields.yaml` — `expected_status: 201` vs `0` default cases.

### Assertions
All **13 assertion types** are implemented once in the centralized `internal/pkg/assertion/*.go` (single `Registry` for both, wired in `cmd/server/main.go:81` via `assertion.NewDefaultRegistry()`). They are used in three places:

| Location | YAML key | When evaluated | Input |
|---|---|---|---|
| Trigger | `triggers[].response_assertions` | Immediately after the trigger HTTP response | Flattened response body (`httputil.FlattenJSON` lowercased) + `Raw` (`receiver/api/receiver.go:174` / `trigger/http.go:136`) |
| Receiver | `triggers[].receivers[].assertions` | After `Collect` returns `domain.Message` (`orchestrator.go:373`) | `Message.Fields` / `Raw` from the collector (`providers/generic.go:49` for webhook) |
| API poll predicate | `receivers[].response_assertions` (`type: api` only) | Each poll, retried until `timeout` | Same as trigger — flattened polled body + `Raw` |

`field`/`value` support `{{variable}}` substitution (run vars, `variables:`/`extract`/`env`) — `orchestrator.go:374` for receivers, `assertion/registry.go:54` for triggers.
**Assertion types:**

| Type | `field` syntax | Passes when |
|------|---------------|-------------|
| `equals` | plain field (lowercased) | `actual == value` |
| `contains` | plain field | `value` is substring of `actual` |
| `not_contains` | plain field | `value` not in `actual` |
| `present` | plain field | field exists and `actual != ""` |
| `matches` | plain field | `actual` matches `value` regex |
| `array_contains` | gjson path (e.g. `items.#.name`) | any element via `gjson.Get(Raw, field)` equals `value` (nested arrays) |
| `map_contains` | gjson path with `@values` (e.g. `labels.@values`) | any `@values` element equals `value` |
| `length` | dot-path to array | `Fields[field+".__len__"] == value` |
| `int_eq` | dot-path | `int(Field) == int(Value)` |
| `int_gt` | dot-path | `>` |
| `int_gte` | dot-path | `>=` |
| `int_lt` | dot-path | `<` |
| `int_lte` | dot-path | `<=` |

**Numeric:** `int_*` parse both sides as `int64` (trimmed) — `"10"` > `"9"` numerically; non-integer fails.

**gjson paths for `array_contains`/`map_contains`:**

| Pattern | Meaning |
|---------|---------|
| `items.#.name` | all `name` from `items` array |
| `data.#.statuses.#.general_status` | nested arrays |
| `labels.@values` | all values of dynamic-key object |
| `labels.@values.#.tag` | `tag` of each map value |
| `items.0.name` | indexed access |

Full refs: `internal/pkg/assertion/*.go`, `internal/pkg/assertion/util.go` (WalkFind), https://github.com/tidwall/gjson#path-syntax

**Field namespaces** (`field` is lowercased at ingestion `httputil/payload.go:66`):

| Context | `field` examples | Populated by |
|---|---|---|
| Trigger / API poll | `id`, `data.status`, `items.#.name` | `httputil.FlattenJSON` |
| Webhook receiver | `headers.x-provider`, `query.status`, `body.data.order_id`, `method` | `providers/generic.go:49` — `headers.<name>` / `query.<name>` / `body.<path>` (also bare `data.order_id` compat) / `method` |
| API receiver (post-poll) | `body.data.status`, `headers.content-type` | `receiver/api/receiver.go:206` |
| IMAP receiver | `subject`, `body`, `from` | `receiver/imap` |

Use lowercase (`headers.x-provider` not `headers.X-Provider`). See examples under `triggers[].response_assertions` and `receivers[].assertions` below.

### Response Assertions

Use `response_assertions` inside a trigger to validate the HTTP response body directly — no receiver needed. See [Assertions](#assertions) for all 13 types and field syntax.

### Receiver Assertions (`assertions`)

Use `assertions` inside a receiver to validate the collected `domain.Message`. See [Assertions](#assertions) for all 13 types and webhook/API/IMAP field namespaces.
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

For webhook-based receivers (e.g., `webhook`), the `options` field is not required as those receivers are configured globally in `config.yaml`.

> **Example:** `tests/manual_imap_test.yaml` + `tests/example_welcome_email.yaml` — full `options: {host, port, username, password, mailbox, tls}`; `tests/example_all_fields.yaml` — `imap` + `webhook`/`api` options side-by-side.

### Webhook Receiver (`type: webhook`)

The webhook receiver lets any external provider signal a test run without writing Go code.
It works with two components:

1. **Ingestion** — the provider (or the app under test) POSTs to
   `{{WEBHOOK_BASE_URL}}/webhook/generic` with the run_id in a query param (`?run_id=`),
   header (`X-E2E-Run-ID`), or body field (`run_id`). 
   
   The server captures the full request — headers, query params, body and deposits it into the store.

2. **Collection** — the receiver polls the store (`Claim(runID, "webhook")`) every 1s
   within the configured `timeout` budget, then runs `assertions` against the captured
   fields.

All assertion field namespaces and types are documented centrally in [Assertions](#assertions).

**Run ID resolution order:**

1. Query param `?run_id=<value>`
2. Header `X-E2E-Run-ID: <value>`
3. Body field `run_id` or `runid`
4. No match → 400

**Example — minimal (`headers` / `query` / `body`):**

```yaml
triggers:
  - method: POST
    url: "{{env.app_base_url}}/api/orders"
    body:
      product_id: "sku-42"
      message_id: "{{run_id}}"
    receivers:
      - type: webhook
        timeout: 60s
        assertions:
          - type: equals
            field: headers.x-provider
            value: "acme"
          - type: equals
            field: query.status
            value: "confirmed"
          - type: contains
            field: body.message_id
            value: "{{run_id}}"
    wait_for_receivers: true
```

The app must callback to:

`POST {{WEBHOOK_BASE_URL}}/webhook/generic?run_id={{run_id}}`

For the full field reference (`headers.*` / `query.*` / `body.*` / `method`) and the 13 assertion types see [Assertions](#assertions).

> **Tip `async` + `wait_for_receivers`:** if the webhook has a long `timeout`, set `async: true` at the test level. `POST /run` will return `202 {run_id, status:"running"}` instantly even if the trigger has `wait_for_receivers: true`; the `wait` continues in background and you poll `GET /results/{run_id}`. See [Async Mode](#async-mode-asynctrue).

> **Example:** `tests/example_generic_webhook.yaml` — `type: webhook` with `headers.x-provider`/`query.status`/`body` assertions; `tests/local_loop_test.yaml` — self-contained loop via `POST /webhook/twilio`; `tests/example_all_fields.yaml` — all webhook field paths.

For providers that embed the correlation token in a nested JSON path, write a custom
extractor and register it in `main.go` under a provider-specific path
(e.g. `POST /webhook/provider_name`). Actual valid ones: twilio, meta & generic

### API Receiver (`type: api`)

There are **two** "API receiver" flows:

| Flow | Type | Semantics |
|------|------|-----------|
| **Webhook / home-delivered** | `webhook` (existing) | A provider pushes a message to the webhook server; the receiver polls the store until it arrives or times out. |
| **Outbound polling** | `api` (new) | The receiver **makes the HTTP call itself**, repeatedly, until the response satisfies the predicate or the budget (`timeout`) expires. |

The `type: api` receiver behaves like a **trigger that polls**: every `interval`
it executes an HTTP request and succeeds when `expected_status` and the
trigger-style `response_assertions` both pass. Failures are **transient** — only
the `timeout` fails the run, so it composes naturally on an eventual-consistency
verification step.

```yaml
triggers:
  - method: POST
    url: "{{env.orders_api}}/checkout"
    body: { order_id: "{{order_id}}" }
    receivers:
      - type: api
        interval: 5s
        timeout: 60s
        method: GET
        url: "{{env.orders_api}}/orders/{{order_id}}"
        headers:
          Authorization: "Bearer {{env.API_TOKEN}}"
        expected_status: 200
        response_assertions:              # poll predicate — retried until timeout
          - type: equals
            field: data.status
            value: "paid"
        assertions:                        # receiver assertions — run once after predicate passes
          - type: equals
            field: body.data.order_id
            value: "{{order_id}}"
          - type: contains
            field: body.data.status
            value: "paid"
    wait_for_receivers: true
```

> **Poll vs receive:** `response_assertions` decide *when to stop polling* (retried); `assertions` decide *whether the final message is correct* (failed once). Field namespaces and the 5 receiver assertion types are documented in [Assertions](#assertions).

**Fields:**

- `interval` — how often to poll (Go duration, e.g. `5s`). Default `5s`.
- `timeout` — overall budget for polling; after it, the receiver returns
  `ErrTimeout` and the test fails. Required unless you want the run's own
  deadline to govern.
- `method` (default `GET`), `url` (required), `headers`, `body` — the polled
  request. `body` is serialized as JSON unless `Content-Type:
  application/x-www-form-urlencoded` is set (same rules as triggers).
- `expected_status` — when set (> 0), the attempt passes **only** if the status
  matches exactly; when unset, any 2xx/3xx passes and 4xx/5xx is a transient
  failure.
- `response_assertions` — the trigger assertion types (`equals`, `contains`,
  `present`, `array_contains`, `int_gt`, …) evaluated against the flattened
  JSON body of **each** poll. Values support `{{variable}}` substitution.
- `{{variable}}` substitution works in `url`, `headers`, `body` and assertion
  values — the receiver receives the full run variables (from `variables:`,
  prior triggers' `extract`, and `run_id`).
- `extract` — accepted by the schema but **not** yet merged back into run
  variables (planned separately).
- `assertions` — message-style assertions run by the orchestrator **after** the
  polling predicate passes, against the flattened response body.

On success the receiver returns the response as a `domain.Message` (`Headers`,
`Fields` flattened from the JSON body, `Raw` body), so message-style `assertions`
keep working as with any other receiver.

> **Example:** `tests/example_api_polling.yaml` — `type: api` polling every `5s` until `response_assertions` pass; `tests/example_all_fields.yaml` — `api` + `webhook` receivers in the same trigger.

### on_failure Block

When a test ends in `failed` or `error` (after all retries are exhausted), the framework notifies the configured alerting endpoints. The `on_failure` block accepts a list of `calls` — outbound HTTP requests executed **sequentially**, one after another:

```yaml
on_failure:
  calls:
    - method: POST
      url: "https://hooks.slack.com/services/XXX"
      timeout: 10s
      headers:
        Content-Type: application/json
      body:
        text: "Test {{test_id}} failed: {{error}}"
    - method: POST
      url: "https://alerts.company.com/tickets"
      delay_before: 2s
      expected_status: 201
      body:
        test_id: "{{test_id}}"
        run_id: "{{run_id}}"
        error: "{{error}}"
        transaction_id: "{{transaction_id}}"
```

Each call supports the same core fields as a trigger: `method` (default `POST`), `url`, `timeout` (default `15s`), `delay_before`, `headers`, `body` (serialized as JSON; `Content-Type` is set automatically) and `expected_status`.

**Template variables available in `on_failure.calls`:**

| Variable | Source |
|---|---|
| `{{run_id}}` | auto-generated per run |
| `{{test_id}}` | test definition `id` field |
| `{{error}}` | failure context (trigger error or aggregated receiver errors) |
| `{{extracted_var}}` | any variable extracted by a trigger step that completed before the failure |
| `{{env.VAR_NAME}}` | OS environment variable (resolved at config load time) |

**Template generators** (see [Template Generators](#template-generators)) also work here, e.g. `body: { alert_id: "{{uuid()}}" }`.

**Behaviour:**

- Calls run **asynchronously** — the failure result is returned to the caller first and the notification never blocks or alters the result.
- If a call references a variable that was never extracted, that **call is skipped and a warning is logged**; the remaining calls still run.
- A call is logged as failed (non-fatal) if the endpoint returns an HTTP error or a status different from `expected_status`.
- If no call is configured, nothing happens.

> **Example:** `tests/example_welcome_email.yaml` — two `on_failure.calls` (alerts + Slack) with `{{test_id}}/{{run_id}}/{{error}}`; `tests/example_all_fields.yaml` — every `CallAction` field + `{{uuid()}}` in body annotated.

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
# Only keys parsed by internal/pkg/config/config.go are shown.
# See tests/example_all_fields.yaml for the test YAML reference.

version: "1"

server:
  port: 8082           # HTTP API port

auth:
  enabled: true        # Set to false for local development to skip JWT
  jwt_secret: "{{env.JWT_SECRET}}"

store:
  type: redis          # redis | postgres | memory | disabled   (default: redis)
  redis:
    url: "{{env.REDIS_URL}}"
    ttl: 300s          # How long received messages are kept
    username: "{{env.REDIS_USERNAME}}"
    password: "{{env.REDIS_PASSWORD}}"
    cluster_mode: false
  postgres:
    dsn: "{{env.POSTGRES_DSN}}"
    ttl: 300s
  memory:
    ttl: 300s

tests:
  path: "./tests"      # Directory containing YAML test definitions

test_groups:
  ci:
    description: "Pipeline that runs after every merge"
    tests:
      - local_loop_test
      - example_variables
    test_delay: 2s
    skip_fail_test: true
```

### Store backends

The message store used to buffer received messages between the webhook
ingestion and the `webhook` receiver is pluggable. Select the backend with the
`store.type` key (`redis` | `postgres` | `memory` | `disabled`, default `redis`).

| Backend | Purpose |
|---------|---------|
| `redis` | **Default.** Distributed, supports cluster mode. Requires `REDIS_URL`. |
| `postgres` | Distributed, relational. Requires `POSTGRES_DSN` (pgx/v5). Schema is created automatically on startup. |
| `memory` | Single-process, in-memory, mutex-protected. No external dependency. Great for local dev and CI. |
| `disabled` | Fully disables the database (no-op store). `webhook` receivers poll until their timeout. |

Only the section matching the selected backend is read; the others are ignored.
`store.type` may be omitted — it defaults to `redis`, so existing config files
keep working unchanged.

> **Running without a DB:** set `store.type: disabled` (or `STORE_TYPE=disabled`). The
> service starts with **no** database dependency. Any test that uses a
> `webhook` receiver will time out waiting for a message, which is
> expected for DB-less runs — use this for pure HTTP-trigger tests.

### Environment variables

| Variable | Required | Used in | Description |
|----------|----------|---------|-------------|
| `REDIS_URL` | Yes (if `store.type: redis`) | `config.yaml` | Redis connection URL (e.g. `redis://localhost:6379`) |
| `POSTGRES_DSN` | Yes (if `store.type: postgres`) | `config.yaml` | PostgreSQL connection DSN (pgx/v5 format) |
| `STORE_TYPE` | No | `config.yaml` | Overrides `store.type` (`redis`/`postgres`/`memory`/`disabled`) |
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

### ✅ 7. gjson-powered `array_contains` + `map_contains`
`array_contains` rewritten using [gjson](https://github.com/tidwall/gjson) path syntax. Supports nested arrays (`data.#.statuses.#.general_status`), flat arrays (`tags`), and dynamic-key objects via `map_contains` (`labels.@values`). Assertion failures now show the raw JSON body instead of the internal flat map.

### 8. Hexagonal Architecture — IngestUseCase Port (Tech Debt)
The `WebhookServer` currently calls `store.Deposit` directly, bypassing the domain layer. A `ports.MessageIngestor` interface and `services.Ingestor` use case should be introduced so all ingestion logic (validation, enrichment, routing) has a single place.

### 9. Dynamic Hot-Reload
Test YAML files are loaded once at startup. Use `fsnotify` to reload `tests/*.yaml` on change (local mode) or expose a `POST /system/reload` endpoint for CI/CD and Git webhook integration.

### 10. Result Persistence
Replace the in-memory `map[string]*domain.TestResult` (max 100 entries, lost on restart) with a durable store. Proposed: Redis with a JSON blob per `run_id` plus a `ZSET` for chronological listing, and a configurable TTL.

### 11. Improve API JSON Response Messages
Standardise all error responses to return `Content-Type: application/json` with a consistent body:
```json
{ "code": 401, "message": "unauthorized" }
```
Currently `http.Error` returns `text/plain`, which is inconsistent with the JSON success responses.

### 12. Production-Ready Console Logging System
Implement a reworked, structured logging system (e.g., using `log/slog`) that outputs strictly to standard console (stdout/stderr). This ensures logs are properly captured, parseable, and fully functional when the project is deployed in production environments like containers, Kubernetes, or any other cloud-native orchestrator.

### 13. Comprehensive Documentation & YAML Reference
Review and enhance the `README.md` documentation. The primary goal is to thoroughly document each feature and rule of the framework strictly from the perspective of the YAML configuration file, providing clear examples and use cases for end-users to understand how to leverage all capabilities.

### ✅ 14. Test-Level `variables:` Block
Tests can define reusable values in a top-level `variables:` map (static aliases and one-shot generators such as `{{uuid()}}` and `{{randomInt(N)}}`). Each value is resolved **once** at run start and stays stable for the whole test, including `on_failure` calls. Reserved names (`run_id`, `test_id`, `error`) are ignored with a warning, and an `extract` value overrides a `variables:` value on name collision. See [The `variables:` Block](#the-variables-block).

---

## Changelog

See [CHANGES.md](CHANGES.md) for the full history of changes.
