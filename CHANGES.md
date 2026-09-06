# CHANGES.md

All notable changes to this project will be documented in this file.
The format follows a chronological order, newest changes first.

---

## [2026-09-06] — Named test groups (`test_group`) for `/run-sequence`

- **New config block** (`configs/config.yaml`): `test_groups` maps a name to a `TestGroupConfig` (`description`, ordered `tests` list, optional `test_delay` and `skip_fail_test` defaults). Parsed by `internal/pkg/config/config.go`.
- **Validation at startup** (`config.ValidateTestGroups`, called from `main.go`): every group must have at least one test, and every referenced test id must resolve to a loaded `TestDefinition` — otherwise the service fails fast to avoid silent pipeline gaps.
- **`ports.GroupResolver`** (`internal/core/ports/group_resolver.go`) + `services.GroupResolver` implementation (`group_resolver.go`): keeps the API adapter decoupled from config internals (Option B). Injected into `api.NewServer` via `api.Config.Resolver`.
- **`POST /run-sequence` now accepts three body shapes** (`internal/adapters/primary/api/server.go`):
  1. Legacy raw array `["a","b"]` (unchanged, backward compatible).
  2. Object `{"test_ids":["a","b"]}`.
  3. Object `{"test_group":"ci"}` — expands the configured group; unknown group → `404`.
  - `test_group` + `test_ids` together → `400`; empty resolved list → `400`.
  - `test_delay`/`skip_fail_test` precedence: explicit query params always win → else group defaults → else built-in defaults.
- **Swagger regenerated** (`swag init`) for the new `/run-sequence` body schema.
- **Tests**: API handler group tests (object `test_ids`, group success, unknown group 404, mutual exclusion 400, query-overrides-group-defaults), and config tests (group parsing + `ValidateTestGroups` empty/unknown cases).
- **Docs**: README "Run a test group" section (three body shapes + CI/CD story + `test_groups` reference) and `configs/config.example.yaml` example.

---

## [2026-09-06] — Pluggable store backends (Redis / PostgreSQL / In-Memory / none)

- **`StoreRegistry` factory** (`internal/adapters/secondary/store/registry.go`): mirrors the trigger registry — `NewStoreRegistry`, `Register(type, factory)`, `Create(type, cfg)` with `domain.ErrConfiguration` for unknown types. Adding a backend is now a new file + one `Register` line in `main.go` (Open/Closed).
- **New backends** behind the same `ports.Store` contract:
  - **PostgreSQL** (`store/postgres.go`, new dependency `github.com/jackc/pgx/v5`): schema (`e2e_messages`, `e2e_reservations`) created idempotently on startup; idempotent `Deposit` (ON CONFLICT), non-deleting `Claim`, NX `Reserve`, `Release`/`Delete`, plus opportunistic purge of expired rows. Guarded by `//go:build integration`.
  - **In-memory** (`store/memory.go`): single-process, mutex-protected maps reproducing the Redis semantics (deposit/claim, TTL expiry, reservation NX conflict + TTL, release/delete).
  - **none** (`store/none.go`): `NoopStore` fully disables the DB; `Claim` returns `(nil, nil)` so `request` receivers poll until their timeout — expected behaviour for DB-less runs.
- **Config** (`config.go`): `store.type` (`redis`/`postgres`/`memory`/`none`, default `redis`), new `store.postgres` and `store.memory` sections, and a `STORE_TYPE` env override. Empty `store.type` defaults to redis (backward compatible). No connection is attempted when `type` is `none`.
- **Wiring** (`cmd/server/main.go`): the store is built via the registry from `cfg.Store.Type`; the resulting `ports.Store` flows unchanged into the webhook server, `request` receiver and orchestrator.
- **Tests**: `store/registry_test.go` (register/create, unknown type, error propagation), `store/memory_test.go` (semantic parity with Redis), `store/none_test.go` (all no-ops), `store/postgres_test.go` (integration), and `config_test.go` (store defaults + full parse + `STORE_TYPE` override).
- **Docs**: README "Store backends" section + env var table (`POSTGRES_DSN`, `STORE_TYPE`), `CONTRIBUTING.md` "Adding a new store backend" guide, `configs/config.example.yaml`, and an optional `postgres` service in `docker-compose.yml`.

---

## [2026-08-31] — Trigger registry (`type`/`options` per step)

- **New `TriggerRegistry`** (`internal/adapters/secondary/trigger/registry.go`): factory pattern mirroring `receiver.ReceiverRegistry` — `NewTriggerRegistry`, `Register(typeName, factory)`, `Create(typeName, options)` with `domain.ErrConfiguration` for unknown types.
- **`TriggerConfig` gained `type` and `options`** (`domain/test.go`): `EffectiveType()` defaults empty `type` to `domain.HTTPTriggerType` (`"http"`). Both fields are inert on the bundled `http` trigger (its factory uses the shared response-assertions registry and ignores `options`, documented as reserved/extensible).
- **Orchestrator now creates the trigger per step** (`orchestrator.go`): the injected `ports.Trigger` is replaced by `*trigger.TriggerRegistry`; `executeSequential` calls `o.triggers.Create(step.EffectiveType(), step.Options)` before the retry loop. An unregistered trigger type fails the step with a clear `unknown trigger type` error (steps after it do not run, `on_failure` still fires). Existing behavior (retry, `delay_before`, receivers, extract, assertions, on_failure) is preserved.
- **Wiring** (`cmd/server/main.go`): `triggerReg.Register(domain.HTTPTriggerType, ...)` builds the HTTP trigger; `NewOrchestrator` receives the registry instead of a trigger instance.
- **New tests**: `trigger/registry_test.go` (register/create, unknown type → `ErrConfiguration`, factory error propagation, options passthrough), `domain/trigger_test.go` (EffectiveType + YAML decode of `type`/`options`), `orchestrator_test.go` (`TestRunSequence_UnknownTriggerTypeFailsStep` — step fails, trigger not executed).
- **Docs**: README trigger section (type/options + how to add a new trigger type), skill trigger bullet, and `docs/PLAN-trigger-registry.md`.

---

## [2026-08-31] — Refactor assertions to registry pattern (receiver & trigger)

- **Receiver assertions moved** from `internal/adapters/secondary/assertion` (package `assertion`) into `internal/adapters/secondary/receiver` (package `receiver`). `AssertionRegistry` renamed to **`ReceiverAssertionRegistry`** (`NewReceiverAssertionRegistry`); consumers (`main.go`, orchestrator, tests) now import the `receiver` package.
- **Trigger `response_assertions` refactored from a `switch` to a registry**: new `TriggerAssertionRegistry` (`NewTriggerAssertionRegistry`, `Register`, `Create`, `Run`) in `internal/adapters/secondary/trigger`. Each assertion is now a factory (`NewEqualsAssertion`, `NewContainsAssertion`, `NewNotContainsAssertion`, `NewPresentAssertion`, `NewMatchesAssertion`, `NewArrayContainsAssertion`, `NewMapContainsAssertion`, `NewLengthAssertion`, `NewIntEqAssertion`, `NewIntGtAssertion`, `NewIntGteAssertion`, `NewIntLtAssertion`, `NewIntLteAssertion`) evaluating a `ResponseAssertionContext` (field, resolved value, flattened response, raw body).
- `HTTPTrigger` now receives the registry as constructor argument (`NewHTTPTrigger(registry)`); passing `nil` falls back to the registry with all built-in assertions registered. `main.go` wires both registries explicitly.
- **Behavior preserved**: same assertion semantics, same error messages/wrapping (`ErrTriggerFailed` + raw body), same variable substitution in expected values. Adding a new response_assertions type is now open/closed: a new factory file plus one `Register` line in `main.go`.

---

- **New int response assertions**: `int_eq`, `int_gt`, `int_gte`, `int_lt`, `int_lte` added to trigger `response_assertions`. Numeric comparison (not string); both `field` value and `value` are parsed as 64-bit integers (whitespace-trimmed). If either side is not an integer, the assertion fails with a clear message. Existing string assertions (`equals`, `contains`, …) are untouched.
- **New stateful operator `{{++(var)}}` / `{{--(var)}}`**: evaluated at the very start of `template.ReplaceString`. Replaces the token with the **already incremented/decremented** value and **persists** the mutation in `vars` (shared by reference through trigger steps, so counters advance across `extract`/`variables` values and later steps). Two evaluation moments: **before the request** (url/headers/body using vars from `variables:` or previous extracts) and **after the request** in later steps (on a var extracted by a prior trigger).
- **Undeclared var**: an **undefined** variable is treated as `0` and created by the operator — `{{++(seq)}}` resolves to `1` (leaving `seq=1`), `{{--(seq)}}` resolves to `-1`. A variable that **exists but is not an integer** is left unresolved (`HasUnresolved` → `true`), no panic, no mutation. Consumers: trigger aborts with a clear `domain.ErrTriggerFailed`, `on_failure.calls` skips the call, assertions simply don't match.
- **Unresolved guard in HTTP trigger**: `template.Execute` now checks `HasUnresolved` over the resolved url, each header and the serialized body **before** `client.Do`, aborting with `domain.ErrTriggerFailed` and the offending field instead of sending a literal `{{...}}` to the API.
- **New tests**: `template_test.go` — sequential increments (1,2,3…), decrement, undefined var defaulting to `0` (created: `{{++}}` → 1,2 and `{{--}}` → -1), non-integer var left unresolved, persistence, and updated-value visibility in the same string; `trigger/http_test.go` — pass/fail per int operator, numeric-vs-string comparison, non-integer failures, variable substitution, trigger abort on unresolved increments (existing **non-integer** var) in url/body, and an undefined increment var being created and sent as `1`.
- **Docs**: `README.md` — int assertion table + example in "Response Assertions" and a new "Increment/Decrement Operator" subsection; `e2e-test-writer` skill — int comparators, operator rules and Quality Checklist items.

---

## [2026-08-29] — Variables section & yaml anchor usage docs

- **New variables section**: This section allows you to create a shared vars across all test steps.
- **Yaml anchor docs**: Documentation about how to use anchors to avoid duplication contennt in test definition.

---

## [2026-08-09] — `{{uuid()}}` generator & extensible template generator registry

- **New generator**: `{{uuid()}}` — resolves to a random UUID v4 (via `github.com/google/uuid`). Works everywhere template resolution happens: URLs, headers, bodies, assertions and `on_failure.calls`.
- **New dependency**: `github.com/google/uuid` v1.6.0.
- **New subpackage**: `internal/pkg/template/generators` owns the whole generator concern — the `Generator` interface (`Name()`, `Generate(args)`), a package-level registry (`Register`, `Resolve`) and every built-in generator (`random_int.go`, `uuid.go`). `internal/pkg/template` keeps only the string-resolution API (`ReplaceString`, `ReplaceMap`, `ReplaceHeaders`, `HasUnresolved`) and delegates lookup to `generators.Resolve`.
- **Self-registration**: each generator lives in its own file and registers itself via `init()`. Adding a new generator is now a single new file in `generators/` that implements `Generator` and calls `Register(...)` — no existing code changes (Open/Closed principle).
- **`randomInt` preserved**: `generateRandomInt` moved unchanged into `RandomIntGenerator`; same semantics and tests.
- **Docs**: `README.md` gains a "Template Generators" section and a note in `on_failure.calls`; `e2e-test-writer` skill documents both generators.
- **Tests**: unit tests for the registry (unknown name, override, nil registration, rejected args) and each generator in `generators/`; integration tests via `ReplaceString`/`ReplaceMap` in the `template` package. All existing template tests pass unchanged.

---

## [2026-08-08] — on_failure.calls: sequential failure notifications

- **Breaking change**: `on_failure.webhook` renamed to `on_failure.calls` — a list of outbound HTTP requests executed **sequentially** when a test fails. The old single `webhook:` block is no longer parsed.
- **New type**: `domain.CallAction` (`method`, `url`, `timeout`, `delay_before`, `headers`, `body`, `expected_status`) — mirrors the core fields of a `TriggerConfig`.
- **Renamed adapter**: `notifier.WebhookNotifier` → `notifier.HTTPNotifier` (`internal/adapters/secondary/notifier/webhook.go` → `http.go`). Constructor `NewWebhookNotifier()` → `NewHTTPNotifier()`. The port interface and its generated mock are unchanged.
- **Extracted variables in notifications**: `on_failure.calls` now resolves every variable extracted by trigger steps that completed before the failure (`{{extracted_var}}`), plus `{{run_id}}`, `{{test_id}}` and `{{error}}`. The orchestrator now populates `TestResult.TriggerVars` on failure paths too (previously only on success).
- **Skip-on-missing-variable**: a call that references a variable that was never extracted is skipped and logged as a warning; the remaining calls still run. New helper `template.HasUnresolved` detects leftover placeholders.
- **`{{error}}` populated on receiver failures**: `collectAndAssertAll` now aggregates failed/errored receiver results into `TestResult.Error` when no trigger error was recorded.
- **Async & non-blocking**: failure notifications run in a detached goroutine (`context.Background()`), so the result is returned to the caller first and the notification never affects result display or timing. Unset call timeouts default to `15s`.
- **`expected_status` per call**: optional exact status match; a mismatch or any 4xx/5xx is logged as a non-fatal failure.
- **Removed**: `{{failed_receivers}}` — documented but never implemented; removed from docs and the example test.
- **Docs**: `README.md` `on_failure` section and `.claude/skills/e2e-test-writer.md` updated to the new syntax.
- **Tests**: `template_test.go` — `HasUnresolved` cases; new `notifier/http_test.go` — skip/failure/substitution/sequencing cases; `orchestrator_test.go` — `TriggerVars` populated on failure.

---

## [2026-07-17] — POST /run-sequence: Sequential Test Execution

- **New endpoint**: `POST /run-sequence` — accepts an ordered JSON array of test IDs as the request body and executes them sequentially, waiting for each to complete before starting the next.
- **Query param `test_delay`**: Optional duration string (e.g. `"2s"`). Sleeps between tests — not before the first.
- **Query param `skip_fail_test`**: Optional boolean (default `false`). When `true`, the sequence stops after the first failed or errored test, returning partial results.
- **New type**: `services.SequenceConfig{Delay, SkipFailTest}` — groups all sequence-level config in one struct, avoiding scattered boolean parameters.
- **New method**: `Orchestrator.RunSequence(ctx, defs, cfg SequenceConfig) []*domain.TestResult` — synchronous sequential runner; blocks on each test result before starting the next.
- **New tests**: `internal/core/services/orchestrator_test.go` — 9 unit tests covering empty input, single test, disabled test, order preservation, delay timing, and SkipFailTest flag.
- **New tests**: `internal/adapters/primary/http/server_test.go` — 11 unit tests covering method validation, JSON parsing, query parameter parsing, unknown IDs, result storage, and failure behavior.
- **Swagger**: Added godoc annotations to `handleRunSequence` for OpenAPI documentation.

---

## [2026-07-14] — `array_contains` rewrite with gjson + `map_contains`

- **New dependency**: `github.com/tidwall/gjson` v1.19.0 for JSON path queries in response assertions.
- **Breaking change**: `array_contains` field syntax changed from `items[].path` to native gjson path syntax (`items.#.path`). Supports flat arrays (`tags`), nested arrays (`data.#.statuses.#.general_status`), and any depth via recursive walk.
- **New assertion type**: `map_contains` — passes if any value in a dynamic-key object equals `value`. Uses gjson `@values` modifier (e.g. `field: "labels.@values"`).
- **Improved error messages**: assertion failure now shows the raw JSON response body instead of the internal flat map. Human-readable even for empty-array cases.
- **New helper**: unexported `walkFind(gjson.Result, target)` recursively traverses nested array results so `data.#.statuses.#.general_status` transparently finds leaf values without requiring `|@flatten`.
- **New tests**: `internal/adapters/secondary/trigger/http_test.go` — 35 tests covering all assertion types, nested arrays, map wildcards, error message regression, and variable substitution.

---

## [2026-07-13] Recursive test loader & triger improvements

- **Recursive test loader**: `internal/pkg/config/loader.go` switched from `os.ReadDir` (flat) to `filepath.WalkDir` so subdirectories under `tests/` are scanned automatically.
- **`expected_status` assertion**: New field `ExpectedStatus int` on `TriggerConfig` (`yaml:"expected_status"`). When non-zero, the trigger fails if the HTTP response code does not match exactly. When zero, the default behaviour (fail on 4xx/5xx) is preserved.
- **`response_assertions` on triggers**: New field `ResponseAssertions []AssertionConfig` on `TriggerConfig`. Asserts fields in the trigger's own response JSON body before continuing to the next step. Sup
ports types: `equals`, `contains`, `not_contains`, `present`, `matches`, `array_contains`, `length` (see below). Assertion error includes the full flattened response body for debugging.
- **`array_contains` assertion type**: Field syntax `"items[].path"` — passes if any element in the array has the nested field equal to `value`. Supports dot-path nesting (e.g. `orders[].address.city`).
- **`length` assertion type**: Asserts exact element count of an array field. `flattenMap` now stores `field.__len__` for every `[]any` it processes (`internal/pkg/httputil/payload.go`).
- **Array index flattening**: `flattenMap` now recurses into `[]any` producing `field.0.key`, `field.1.key`, … — accessible via dot-notation in `extract` and all `response_assertions` types.
- **`delay_before` per step**: New field `DelayBefore time.Duration` on `TriggerConfig` (`yaml:"delay_before"`). Sleeps once before the first attempt of that step (not before each retry).
- **`attempts` in API response**: `result.Attempts` now incremented on each trigger attempt; `TestResult` and `ReceiverResult` fields use snake_case JSON tags.

---
## [2026-07-11] — Unified Triggers (Multiple Steps)

- **Unified syntax**: `TestDefinition.Triggers []TriggerConfig` (YAML key: `triggers`) is the only supported syntax. Each trigger groups an HTTP call with its own receivers and a `wait_for_receivers` flag.
- **Breaking change**: The legacy `trigger:` + top-level `receivers:` syntax has been removed. All test definitions must use `triggers:` (plural) with receivers inline per trigger.
- **Orchestrator simplified**: `execute()` always delegates to `executeSequential()`. The legacy code path has been removed. Retry logic applies per trigger.
- **`wait_for_receivers` flag**: When `true`, the orchestrator starts the trigger's receivers, executes the HTTP call, collects and asserts before moving to the next trigger. When `false`, only the HTTP call fires and variables are extracted.
- **Variable accumulation**: Variables extracted in earlier triggers are available in later triggers via `{{variable_name}}`.
- **Result enrichment**: `ReceiverResult` now includes `TriggerIndex int` field to identify which trigger produced the result.

---
## [2026-05-12] — Roadmap update and Domain Error Wrapper

- **Documentation**: Updated `README.md` roadmap. Reformulated observability to point 9 as "Production-Ready Console Logging System", and added point 10 "Comprehensive Documentation & YAML Reference".
- **New package**: Added `errorwrapper` package in `internal/pkg/errorwrapper/wrapper.go` with `Wrap(domainErr, err)` helper to standardise error wrapping without redundant messages.
- **Refactored**: Updated `internal/adapters/secondary/receiver/imap/client.go` to use `errorwrapper.Wrap` instead of inline `fmt.Errorf`.

---
## [2026-05-10] — OptionsMap: native YAML types in receiver options

- **New type**: `domain.OptionsMap` (`map[string]string` with custom `UnmarshalYAML`) in `internal/core/domain/test.go`. Allows `bool`, `int`, `float` in YAML `options` blocks — all normalized to `string` transparently. Existing adapters unchanged.
- **Updated**: `tests/example_welcome_email.yaml` — `tls: true` now uses native YAML boolean instead of quoted string.

## [2026-05-10] — IMAP Receiver Implementation (Roadmap Point 5)

- **New dependency**: `github.com/emersion/go-imap/v2`, `github.com/emersion/go-imap/v2/imapclient`, `github.com/emersion/go-message`.
- **New dependency**: `go.uber.org/mock/gomock` + `mockgen` CLI for generated test mocks.
- **New port**: `internal/core/ports/imap_client.go` — `IMAPClient` interface (`Connect`, `SearchByRunID`, `Disconnect`).
- **New adapter**: `internal/adapters/secondary/receiver/imap/client.go` — `GoIMAPClient` implementing `IMAPClient` via `go-imap/v2`. Searches `runID` first in Subject header, then in Body.
- **New file**: `internal/adapters/secondary/receiver/imap/parser.go` — pure `parseMessage` function using `go-message/mail`. Extracts `text/plain`, `text/html`, all headers and date into `domain.Message`.
- **Refactored**: `internal/adapters/secondary/receiver/imap/receiver.go` — `NewIMAPReceiver` now wires `GoIMAPClient` from options map. Removed all TODO and TEMP placeholder code.
- **New mocks**: `internal/core/ports/mocks/` — generated mocks for all 7 port interfaces (`Assertion`, `Extractor`, `IMAPClient`, `Notifier`, `Receiver`, `Store`, `Trigger`).
- **New tests**: `internal/adapters/secondary/receiver/imap/receiver_test.go` — 12 unit tests using `gomock` EXPECT covering full lifecycle (constructor validation, Start, Collect, Stop).
- **New tests**: `internal/adapters/secondary/receiver/imap/client_test.go` — 8 unit tests for `parseMessage` covering plain text, multipart, HTML-only, Q-encoded subject, named from, headers, date and malformed input.
- **Config**: `tests/example_welcome_email.yaml` — IMAP `options` block updated to use `{{env.IMAP_HOST}}`, `{{env.IMAP_PORT}}`, `{{env.IMAP_USERNAME}}`, `{{env.IMAP_PASSWORD}}` env vars. IMAP credentials are per-test, not global config.
- **Makefile**: Added `make mocks` target wrapping `go generate ./internal/core/ports/...`.
- **README**: Added `## Generate Mocks` section documenting `make mocks`.

---
## [2026-05-09] — Security: JWT Authentication (Roadmap Point 3)

- **New dependency**: `github.com/golang-jwt/jwt/v5`.
- **New error**: `domain.ErrUnauthorized` added to `internal/core/domain/errors.go`.
- **New package**: `internal/pkg/auth/jwt.go` — `Claims` struct (`Provider string` + `jwt.RegisteredClaims`) and `ValidateToken(tokenStr, secret string) (*Claims, error)`.
- **Config**: Added `auth.enabled` and `auth.jwt_secret` (via `{{env.JWT_SECRET}}`) to `config.go` and `configs/config.yaml`. Defaults to `enabled: false` for backward compatibility.
- **HTTP API** (`internal/adapters/primary/http/server.go`): Added `authMiddleware` validating `Authorization: Bearer <JWT>`. Protects `/run`, `/results`, `/results/`, `/swagger/`. `/health` remains public.
- **Webhook Server** (`internal/adapters/primary/webhook/server.go`): Validates JWT from `?token=<jwt>` query param. Both servers log `sub` and `provider` claims on authenticated requests.
- **Wiring** (`cmd/server/main.go`): Both `NewServer` calls updated to pass `cfg.Auth.Enabled` and `cfg.Auth.JWTSecret`.

---
## [2026-05-03] — Retry Logic (Roadmap Point 2)

- **Feature**: Implemented retry logic in `internal/core/services/orchestrator.go`. The orchestrator now reads `def.Retry.Enabled`, `def.Retry.Attempts` and `def.Retry.Delay` from the YAML definition.
- **Changed**: `execute()` refactored — recipient reservations are made **once** before the retry loop and released via `defer` after all attempts. Receivers are created, started and stopped on each individual attempt. `on_failure` notification is only sent after all attempts are exhausted.
- **Domain**: Added `Attempts int` field to `domain.TestResult` to record the total number of execution attempts.
- **Behaviour**: Configuration errors (failed to create/start a receiver) abort the retry loop immediately and are not retried. Trigger failures and collection/assertion failures are retried up to `attempts` times with `delay` between each.

Example YAML:
```yaml
retry:
  enabled: true
  attempts: 3
  delay: 5s
```

---
## [2026-05-03] — Receiver Options & IMAP Skeleton

- **Feature**: Added `Options map[string]string` field to `domain.ReceiverConfig` (YAML key: `options:`). Allows each test to pass receiver-specific configuration (e.g., IMAP host, port, credentials) directly in the YAML without any code changes.
- **Changed**: `ReceiverFactory` signature updated from `func() ports.Receiver` to `func(options map[string]string) (ports.Receiver, error)`. The registry now passes the YAML options to the factory at creation time.
- **Feature**: Added `IMAPReceiver` skeleton in `internal/adapters/secondary/receiver/imap/receiver.go`. Reads `host`, `port`, `username`, `password`, `mailbox`, and `tls` from the options map. Marked with `TODO` where the real IMAP client will be injected.
- **New Port**: Added `ports.IMAPClient` interface in `internal/core/ports/imap_client.go`.
- **Updated**: `README.md` — added `options:` field documentation and IMAP receiver example.

---

## [2026-05-01] — Step 19: Clean Code, Linter & Domain Errors

- **Coding Standards Enforcement**: Created `CODING_STANDARDS.md` documenting strict Go programming rules for the project.
- **Domain Errors**: Created `internal/core/domain/errors.go` with predefined errors (`ErrConfiguration`, `ErrInternal`, `ErrTimeout`, `ErrTriggerFailed`, `ErrValidation`).
- **Error Wrapping Refactor**: Refactored over 15 files across all secondary adapters to wrap domain errors (e.g. `fmt.Errorf("%w: timeout: %v", domain.ErrTimeout, err)`) instead of using flat string errors, improving traceability and enabling `errors.Is`.
- **Error Handling Pattern**: Enforced the `if err != nil { if ... }` pattern throughout the codebase, removing nested error checks.
- **Linter Integration**: Added `.golangci.yml` configuring `errcheck`, `govet`, `ineffassign`, `gofmt`, `goimports`, and `whitespace`.
- **Return Formatting**: Ensured a blank line separates the final `return` statement from the preceding logic blocks across the codebase.

---

## [2026-05-01] — Step 18: Variable Injection in Assertions (Production Readiness Phase 1)

- **Variable Injection (Bug Fix)**: The Orchestrator now correctly injects dynamically extracted trigger variables into test assertions.
- Updated `collectAndAssert` in `internal/core/services/orchestrator.go` to accept `triggerVars map[string]string`.
- Used `template.ReplaceString` to evaluate variables (e.g., `{{transaction_id}}`) inside `AssertionConfig.Field` and `AssertionConfig.Value` right before creating the assertion instance.
- This unlocks the ability to assert values that are generated at runtime by the external systems being tested.

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
