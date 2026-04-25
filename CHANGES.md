# CHANGES.md

All notable changes to this project will be documented in this file.
The format follows a chronological order, newest changes first.

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
