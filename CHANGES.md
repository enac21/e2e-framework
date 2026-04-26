# CHANGES.md

All notable changes to this project will be documented in this file.
The format follows a chronological order, newest changes first.

---

## [2026-04-26] — Step 17: Swagger Documentation

- Added Swagger annotations to `cmd/server/main.go` for general API information.
- Added Swagger annotations to all HTTP handlers in `internal/adapters/primary/http/server.go`.
- Registered `/swagger/*` endpoint using `http-swagger` for interactive API documentation.
- Generated Swagger 2.0 documentation using `swag init`.
- Updated `README.md` with instructions on how to access the Swagger UI and how to regenerate the documentation.

---

## [2026-04-26] — Step 16: Orchestrator RunID Ownership & Refactoring

- `Orchestrator.RunTest` now generates the RunID internally and returns `(string, <-chan *domain.TestResult)` immediately, launching execution asynchronously.
- Removed `runID` parameter from `RunTest` — the core is now the sole owner of execution identity.
- HTTP server and Cron scheduler no longer generate IDs; they receive the runID from the Orchestrator and decide whether to block on the channel (sync) or not (async).
- Fixed async mode bug: placeholder and final result now share the same RunID key, making polling correct.
- `def.Async` moves to an adapter concern: the HTTP handler decides to wait on the channel or not.
- Removed `generateRunID` helper from HTTP adapter.
- Removed unused `time` and `fmt` imports from `cron/scheduler.go`.
- All constructors for `Receiver` adapters (`NewSmsReceiver`, `NewWebhookReceiver`, `NewPushReceiver`, `NewEmailReceiver`) now return concrete types instead of `ports.Receiver`.
- `Extractor` interface moved from `adapters/primary/webhook` to `internal/core/ports/extractor.go`.

---

## [2026-04-26] — Step 15: Roadmap Implementation

- **Trigger Data Extraction**: Added `Extract map[string]string` to `domain.TriggerConfig`. `HTTPTrigger.Execute` now returns `(map[string]string, error)`, reading response JSON and extracting values by dot-notation path. `TestResult.TriggerVars` exposes extracted values. `httputil.FlattenJSON` exported for reuse.
- **Async API Execution**: Added `Async bool` to `domain.TestDefinition`. `/run` returns `202 Accepted` with `run_id` and `poll_at` for async tests. New `GET /results/{run_id}` polling endpoint. Added `StatusRunning` to `domain.RunStatus`.
- **Redis Data Cleanup**: Added `Delete` to `ports.Store` interface and implemented in `RedisStore`. Orchestrator calls `Delete` after each receiver successfully collects its message, removing the key immediately instead of relying only on TTL.
- **Recipient Reservation**: Added `Recipient string` to `domain.ReceiverConfig`. Orchestrator calls `store.Reserve` before starting each receiver (if `recipient` is non-empty) and `store.Release` in the deferred cleanup. Prevents concurrent runs from claiming the same channel/recipient.
- Removed legacy `handler_run.go`, `handler_results.go`, `handler_health.go` — all HTTP handler logic consolidated in `server.go`.
- Results store in HTTP server refactored from `[]*TestResult` slice to `map[string]*TestResult` for O(1) lookup by `run_id`.

---

## [2026-04-26] — Step 14: Architecture Cleanup

- Moved `Extractor` interface from `adapters/primary/webhook/extractor.go` to `internal/core/ports/extractor.go`.
- Created `internal/pkg/httputil/payload.go` with `ExtractFields` generic utility: transparently handles `application/json` (with recursive `flattenMap` for nested keys) and `application/x-www-form-urlencoded` (with lowercase key normalization).
- Refactored `TwilioExtractor` and `MetaExtractor` to delegate all payload parsing to `httputil.ExtractFields`.
- `TwilioExtractor` now extracts `runID` as `strings.TrimSpace(fields["body"])` — no prefix parsing.
- `RedisStore` key format unified under `e2eTestKey` constant (`"e2e-test:%s:%s"`).

---

## [2026-04-26] — Step 13: Debugging & Refinement

- Added initialization logs to HTTP API server and Webhook server on startup.
- `HTTPTrigger` updated to detect `Content-Type` header and serialize body as `application/x-www-form-urlencoded` or `application/json` accordingly.
- Fixed `run_id` extraction in `TwilioExtractor`: removed prefix-based substring logic; `Body` field is now used directly as the `runID`.

---

## [2026-04-25] — Step 12: Main Wiring & Graceful Shutdown

- Implemented `cmd/server/main.go` using `golang.org/x/sync/errgroup` to run all primary adapters concurrently
- Loaded configurations and registered all 5 assertion types and 4 receiver types
- Instantiated the `Store`, `Trigger`, and `Notifier` adapters and passed them to the `Orchestrator`
- Implemented robust `SIGINT`/`SIGTERM` signal catching and graceful shutdown for HTTP servers and Cron scheduler

---

## [2026-04-25] — Step 11: Config and YAML Loader

- Implemented `internal/pkg/config/config.go` to parse `configs/config.yaml`
- Implemented `internal/pkg/config/loader.go` to traverse `tests/` and parse `*.yaml` files into `domain.TestDefinition` structs
- Added an environment variable resolver that automatically replaces `{{env.VAR_NAME}}` in raw YAML strings before unmarshaling using `gopkg.in/yaml.v3`

---

## [2026-04-25] — Step 10: Cron Scheduler (Primary Adapter)

- Implemented `adapters/primary/cron/scheduler.go` using `github.com/robfig/cron/v3`
- The scheduler reads the `schedule` property from the YAML definition and triggers the orchestrator
- Runs in a separate goroutine and handles lifecycle (Start/Stop)

---

## [2026-04-25] — Step 9: HTTP API Server (Primary Adapter)

- Implemented `adapters/primary/http/server.go` to expose the REST API
- `GET /health` endpoint for readiness/liveness checks
- `POST /run?id={test_id}` endpoint to trigger manual execution of tests via the `Orchestrator`
- `GET /results` endpoint to fetch in-memory aggregated test results
- Implemented graceful shutdown and thread-safe results array

---

## [2026-04-25] — Step 8: Webhook Server (Primary Adapter)

- Implemented `adapters/primary/webhook/server.go` to receive incoming webhooks
- Created `Extractor` interface and implementations for `twilio` (SMS) and `meta` (Push)
- The webhook server extracts payloads into `domain.Message` and deposits them into the `Store`

---

## [2026-04-25] — Step 7: Orchestrator

- Implemented `Orchestrator` in `internal/core/services/orchestrator.go`
- Orchestrator handles the full lifecycle: initializes receivers, triggers HTTP action, and polls receivers concurrently
- Uses `sync.WaitGroup` to wait for all receiver collections concurrently while respecting per-receiver timeouts
- Aggregates statuses correctly and executes the `Notifier` port if the global status is `failed` or `error`
- Uses `defer` to ensure all receivers are cleanly stopped (`Stop()`) regardless of errors

---

## [2026-04-25] — Step 6: Receiver Adapters

- Implemented `ReceiverRegistry` using the Factory pattern (`func() ports.Receiver`) to ensure each test execution gets a fresh stateful receiver instance
- Implemented 4 receiver adapters: `webhook`, `sms`, `push`, and `email`
- All receivers share the same `Store` polling strategy (`store.Claim` inside a 1-second ticker loop), unifying the architecture around the Redis buffer
- Updated `CONTRIBUTING.md` registration example to use the factory pattern instead of a singleton instance

---

## [2026-04-25] — Step 5: Trigger and Notifier Adapters

- Modified `ports.Notifier` interface to receive `domain.OnFailureConfig` for stateless execution
- Added `internal/pkg/template` for recursive string variable replacement in nested maps and slices
- Implemented `HTTPTrigger` adapter resolving `{{run_id}}` in URL, Headers, and Body before HTTP dispatch
- Implemented `WebhookNotifier` adapter resolving `{{run_id}}`, `{{test_id}}`, and `{{error}}` for failure alerts
- Both adapters use standard `http.Client` with timeout handling and JSON serialization

---

## [2026-04-25] — Step 4: Assertion Adapters

- Implemented `AssertionRegistry` with factory pattern in `assertion/registry.go`
- Implemented 5 assertions: `ContainsAssertion`, `EqualsAssertion`, `MatchesAssertion`, `PresentAssertion`, `NotContainsAssertion`
- Each assertion returns descriptive errors with field name, expected value, and actual value
- `MatchesAssertion` compiles regex at construction time for fail-fast on invalid patterns
- Registry returns `fmt.Errorf` for unknown assertion types, never panics

---

## [2026-04-25] — Step 3: Store Adapter (Redis)

- Implemented `RedisStore` in `adapters/secondary/store/redis.go` implementing `ports.Store`
- Four methods: `Deposit` (SET+JSON+TTL), `Claim` (GET+deserialize, nil on miss), `Reserve` (SetNX atomic), `Release` (DEL)
- Constructor `NewRedisStore(cfg)` accepts `RedisStoreConfig` with URL and TTL
- Added `Close()` for graceful shutdown of Redis client
- New dependency: `github.com/redis/go-redis/v9`

---

## [2026-04-25] — Step 2: Core Domain

- Implemented `domain.Message` (NormalizedMessage) with RunID, ReceiverType, ReceivedAt, Headers, Fields, and Raw
- Implemented `domain.RunStatus` enum with four states: `passed`, `failed`, `error`, `skipped`
- Implemented `domain.ReceiverResult` for per-channel test outcomes
- Implemented `domain.TestResult` for complete test execution results
- Implemented `domain.TestDefinition` and all sub-types: `RetryConfig`, `TriggerConfig`, `ReceiverConfig`, `AssertionConfig`, `OnFailureConfig`, `WebhookAction` — all with YAML struct tags
- Implemented `ports.Trigger` interface with stateless `Execute(ctx, TriggerConfig, runID)` signature
- Implemented `ports.Receiver` interface with `Start`/`Collect`/`Stop` lifecycle
- Implemented `ports.Assertion` interface with `Assert(msg)` returning descriptive errors
- Implemented `ports.Store` interface with `Deposit`/`Claim`/`Reserve`/`Release` methods
- Implemented `ports.Notifier` interface with fire-and-forget `Notify` semantics
- All types and interfaces have complete godoc comments
- Zero imports from `adapters/` in any `core/` file

---

## [2026-04-25] — Step 1: Project Skeleton

- Created full directory structure following hexagonal architecture pattern
- Added `cmd/server/main.go` with wiring TODOs (no business logic)
- Added placeholder files for all core domain models (`message.go`, `result.go`, `test.go`)
- Added placeholder files for all port interfaces (`trigger.go`, `receiver.go`, `assertion.go`, `store.go`, `notifier.go`)
- Added placeholder for `core/services/orchestrator.go`
- Added placeholder files for all primary adapters: HTTP server, webhook server, cron scheduler
- Added placeholder files for all secondary adapters: trigger, receiver (email, sms, push, webhook), assertion (contains, equals, matches, present, not_contains), store (Redis), notifier (webhook)
- Added `Makefile` with build, test, lint, and Docker targets
- Added `Dockerfile` with multi-stage build (Go builder → Alpine runtime)
- Added `docker-compose.yml` with `e2e-service` (port 8080) and `redis` (port 6379)
- Added `configs/config.yaml` with the global configuration schema
- Added `tests/example_welcome_email.yaml` with the test YAML schema
- Added `CONTRIBUTING.md` with 5-step guide for adding new receivers
- Added `README.md` with project overview and architecture summary
