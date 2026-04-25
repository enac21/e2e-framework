# CHANGES.md

All notable changes to this project will be documented in this file.
The format follows a chronological order, newest changes first.

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
