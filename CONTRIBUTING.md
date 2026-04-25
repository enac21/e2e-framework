# Contributing to e2e-testing-service

Thank you for your interest in contributing! This document explains how to extend
the framework with new capabilities.

---

## Adding a New Receiver

Adding support for a new notification channel requires only 5 steps.
No modifications to existing code are needed beyond `cmd/server/main.go`.

### Step 1 — Create the implementation file

Create a new file at:

```
internal/adapters/secondary/receiver/{channel}/{channel}.go
```

For example, for a Slack receiver:

```
internal/adapters/secondary/receiver/slack/slack.go
```

### Step 2 — Implement the `ports.Receiver` interface

Your receiver must implement all three methods defined in `internal/core/ports/receiver.go`:

```go
type Receiver interface {
    Start(ctx context.Context, runID string) error
    Collect(ctx context.Context) (*domain.Message, error)
    Stop() error
}
```

- **`Start()`** — Initialize the receiver for a specific test run. Register interest
  in the store for the given `runID`, open connections, etc.
- **`Collect()`** — Wait for and return a `domain.Message`. This should block until
  a message arrives or the context times out.
- **`Stop()`** — Clean up resources, close connections, release store slots.

Normalize all received data into a `domain.Message` with appropriate fields in the
`Fields` map (e.g., `from`, `to`, `body`, `subject`, etc.).

### Step 3 — Add infrastructure config (if needed)

If your receiver requires external configuration (API keys, connection strings, etc.),
add the necessary fields to `configs/config.yaml` under the `receivers` section:

```yaml
receivers:
  slack:
    token: "{{env.SLACK_BOT_TOKEN}}"
    channel: "{{env.SLACK_CHANNEL}}"
```

Always use `{{env.VAR_NAME}}` for secrets — never hardcode them.

### Step 4 — Register in `main.go`

In `cmd/server/main.go`, instantiate your receiver and register it in the
`ReceiverRegistry`:

```go
slackReceiver := slack.New(config.Receivers.Slack, store, logger)
receiverRegistry.Register("slack", slackReceiver)
```

### Step 5 — Use in test YAML files

Your new receiver is now available in any test YAML definition:

```yaml
receivers:
  - type: slack
    timeout: 30s
    assertions:
      - type: contains
        field: body
        value: "Welcome message"
```

No other files need to be modified. The orchestrator discovers receivers
by their registered type string.

---

## Code Style

- All files must have a `package` declaration and a comment explaining their responsibility.
- Use `go vet` and `go fmt` before submitting.
- Follow table-driven test patterns.
- Never import from `adapters/` inside `core/` — this is an architectural violation.

---

## Running Tests

```bash
# Unit tests
make test

# Integration tests (requires running Redis)
make test-integration
```
