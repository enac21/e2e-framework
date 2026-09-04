# CLAUDE.md

This document is the single source of truth for the coding standards, architecture and
testing conventions of this repository. It applies to every code change — for both
human contributors and AI agents.

---

## 1. Project Overview

`e2e-testing-service` is a **data-driven end-to-end testing framework** written in Go.
Teams define tests declaratively via YAML files — no code changes required. A test
triggers one or more HTTP requests and validates that the expected notifications arrive
through one or more channels (email, SMS, push, webhook, etc.). The orchestrator drives
the full lifecycle: trigger → collect receivings → assert → store → notify on failure.

---

## 2. Architecture (Hexagonal / Clean)

The project follows a **hexagonal (ports and adapters)** architecture.

```
adapters/primary   →   core/services   →   ports   ←   adapters/secondary
```

### Layer map

| Layer | Path | Responsibility |
|---|---|---|
| Domain | `internal/core/domain` | Entities and sentinel errors (`Err*`). No external dependencies. |
| Ports | `internal/core/ports` | Interfaces only — independent of any implementation. |
| Services | `internal/core/services` | Orchestration (`Orchestrator`, `RunSequence`, `RunTest`...). |
| Primary adapters | `internal/adapters/primary` | Entry points: `api`, `webhook`, `cron`. |
| Secondary adapters | `internal/adapters/secondary` | Concrete implementations: `store` (redis), `receiver` (imap/request), `notifier`, `assertions` (receiver/trigger), `trigger` (http). |
| pkg | `internal/pkg` | Utilities without business logic (`template`, `config`, `httputil`, `errorwrapper`). |

### Strict dependency rule

- **`adapters → core`** — adapters import `ports` and `domain`.
- `core` **NEVER** imports `adapters` or any external layer.
- `core` may import `pkg` (utilities); `adapters` may too.
- `pkg` does **not** depend on `core` nor on `adapters` — it is a pure utility layer.

### Data flow

```
trigger → receivers → assertions → store → notifier
```

### Extensibility (Open/Closed)

Adding a new capability = **new file + registration** via `Register`/a registry, without
touching existing code. This pattern is already used by the trigger, receiver, assertion
and generator registries (e.g. `internal/adapters/secondary/receiver/registry.go`,
`internal/pkg/template/generators/registry.go`).

---

## 3. File Structure (every `.go` file)

Mandatory ordering inside each file:

1. `package` declaration.
2. `import` blocks — stdlib first, then external/internal, grouped as goimports/`gofmt`.
3. **Variables and constants** — at the very top, right after imports (e.g. domain
   sentinel errors, reserved-name sets).
4. Type, struct and interface definitions.
5. Constructor(s) `NewXxx`.
6. **Public methods**, grouped top to bottom:
   1. those that **take data** (Read / Get / Collect),
   2. then those that **modify** (Write / Create / Update / Start),
   3. finally those that **delete / release** (Delete / Stop / Close).
7. **Private methods** — in order of use, always after ALL public methods.
8. A comment explaining the file's responsibility. Place it as a package-doc comment
   directly above the `package` declaration (Go convention).

---

## 4. Naming & Code Style

- Public `Foo` → private `foo`; constructor `NewFoo`.
- Clear, consistent verbs: `Get`/`Create`/`Update`/`Delete`/`Start`/`Stop`/`Collect`...
- Short functions with a single responsibility.
- Match gofmt/goimports formatting; comments explain **why**, not what.

---

## 5. Error Handling

- Domain sentinel errors live in `internal/core/domain/errors.go`
  (`ErrConfiguration`, `ErrInternal`, `ErrTimeout`, `ErrNotFound`, `ErrValidation`,
  `ErrUnimplemented`, `ErrUnauthorized`, ...).
- Always **wrap** errors: `fmt.Errorf("%w: ...", domain.ErrX)`.
- In low-level adapters you may use `errorwrapper.Wrap(domain.ErrX, err)`.
- Propagate the domain error as-is; adapters must **not** translate it into their own
  error types — the orchestrator evaluates domain errors.

---

## 6. Testing Standard

- Every **implementation-logic** file has its `_test.go`. Exempt:
  `internal/core/ports` (interfaces), `internal/core/ports/mocks/` and `cmd/.../main.go`.
- **One main `TestXxx` per public method**, named `Test` + method name (`TestCollect`,
  `TestStop`).
- **Inside each `TestXxx`**: every case is a `t.Run("descriptive title", func...)` with
  setup + assertions **inline in the same function** — do not delegate to helpers defined
  elsewhere in the file.
- The `TestXxx` functions appear in the `_test.go` in the **same order** as their
  methods in the `.go` file.
- Use the **table-driven** pattern when inputs are tabular and the body is identical.
- Titles are simple, descriptive scenario labels ("missing host", "times out",
  "connection error").

### Example structure

Applied to `internal/adapters/secondary/receiver/imap/receiver.go`, which exposes the
public methods `NewIMAPReceiver`, `Start`, `Collect`, `Stop`:

```go
package imap

import (
	"context"
	"testing"
)

// One main TestXxx per PUBLIC method.
func TestNewIMAPReceiver(t *testing.T) {
	t.Run("missing host", func(t *testing.T) {
		// setup + assertions INLINE, right here
	})

	t.Run("host without port", func(t *testing.T) {
		// ...
	})

	t.Run("invalid port", func(t *testing.T) {
		// ...
	})

	t.Run("valid options with defaults", func(t *testing.T) {
		// ...
	})
}

func TestStart(t *testing.T) {
	t.Run("connects and stores run id", func(t *testing.T) {
		// ...
	})

	t.Run("connection error", func(t *testing.T) {
		// ...
	})
}

func TestCollect(t *testing.T) {
	t.Run("not started", func(t *testing.T) {
		// ...
	})

	t.Run("returns message when found", func(t *testing.T) {
		// ...
	})

	t.Run("times out", func(t *testing.T) {
		// ...
	})
}

func TestStop(t *testing.T) {
	t.Run("disconnects client", func(t *testing.T) {
		// ...
	})
}
```

**Key points:**
- One `TestXxx` per public method, named `Test` + the exported method name.
- Every case inside is a `t.Run("clear title", ...)` with setup and assertions **inline**;
  no helpers defined elsewhere in the file.
- Titles describe the scenario ("missing host", "times out", "connection error").

---

## 7. Tooling & Verification (mandatory)

Before finishing **any** change:

```bash
go build ./...
go vet ./...
golangci-lint run        # config: .golangci.yml
go test ./...            # or: make test
gofmt / goimports        # already enforced by the lint config
```

The standard applies to new code and to any code/tests you touch. Refactoring existing
code that does not yet follow it is a separate, explicit task.