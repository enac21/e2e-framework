# E2E Framework — Overview

YAML-based E2E testing framework. No test code to write — define a `.yaml`, the service loads and executes it. Written in Go with hexagonal architecture (ports & adapters).

---

## What does it do?

1. Loads test definitions from `tests/*.yaml` on startup. In the near future, it will load tests from another repository.
2. It allows you to launch tests on demand via HTTP or by scheduling the test with a cron job.
3. Fires an HTTP trigger (the action being tested)
4. Wait for receiver verification (email IMAP, webhook...)
5. Validates received messages with declarative assertions
6. Automatically retries on failure
7. Notifies via webhook if the test fails after all retry attempts

---

## Working with YAML (without touching code)

### Workflow

```
1. Create/edit file in tests/my_test.yaml
2. Restart the service (or deploy)
3. Execute: POST /run?id=my_test
4. View result: GET /results/{run_id}
```

For scheduled tests, add `schedule: "*/5 * * * *"` and the scheduler registers them automatically.

### YAML Anatomy

| Field | Description |
|-------|-------------|
| `id` | Unique test identifier |
| `description` | Human-readable description |
| `enabled` | `true/false` — disable without deleting |
| `async` | `false` → 200 response + result; `true` → 202 + polling |
| `schedule` | Cron expression for scheduled execution |
| `retry.attempts` | Total number of attempts (including first) |
| `retry.delay` | Wait between attempts (`5s`, `1m`, etc.) |
| `trigger` | HTTP request that starts the flow (method, url, headers, body) |
| `trigger.extract` | JSON paths from response to extract variables |
| `receivers[]` | List of channels to listen on (`imap`, `request`) |
| `receivers[].assertions[]` | Validations on received messages |
| `on_failure.webhook` | Alert webhook if test fails |

### Dynamic Variables

```yaml
# Available in: trigger body/headers/url, receiver options, assertion values, on_failure body
{{run_id}}              # Unique UUID per execution — correlates trigger with received message
{{env.VAR_NAME}}        # Environment variable
{{extracted_variable}}  # Value extracted from trigger response via trigger.extract
{{test_id}}             # Test ID (available in on_failure)
{{error}}               # Error message (available in on_failure)
```

### Complete Example

```yaml
version: "1"
id: welcome_email
description: "Verify welcome email is sent after registration."

schedule: "*/5 * * * *"
enabled: true

retry:
  enabled: true
  attempts: 3
  delay: 10s

trigger:
  method: POST
  url: "https://api.service.com/users"
  timeout: 10s
  headers:
    Content-Type: application/json
    Authorization: "Bearer {{env.API_TOKEN}}"
  body:
    email: "test@domain.com"
    message_id: "{{run_id}}"
  extract:
    user_id: "data.id"           # Extracts response.data.id as {{user_id}}

receivers:
  - type: imap
    timeout: 60s
    options:
      host: "{{env.IMAP_HOST}}"
      port: "{{env.IMAP_PORT}}"
      username: "{{env.IMAP_USERNAME}}"
      password: "{{env.IMAP_PASSWORD}}"
      mailbox: INBOX
      tls: true
    assertions:
      - type: contains
        field: subject
        value: "Welcome"
      - type: equals
        field: from
        value: "noreply@company.com"
      - type: contains
        field: body
        value: "{{run_id}}"

on_failure:
  webhook:
    url: "https://alerts.company.com/hook"
    method: POST
    body:
      test_id: "{{test_id}}"
      run_id: "{{run_id}}"
      error: "{{error}}"
```

---

## Available Assertions

| Type | Behavior |
|------|----------|
| `contains` | Field contains the expected substring |
| `equals` | Field is exactly equal to the expected value |
| `matches` | Field matches the regex in `value` |
| `present` | Field exists and is not empty |
| `not_contains` | Field does NOT contain the substring |

Available fields in emails: `subject`, `from`, `to`, `date`, `body`, `html_body`  
Fields in webhooks: depends on the extractor (e.g. Twilio: `from`, `to`, `body`)

---

## E2E Flows — Diagrams

### 1. Service Bootstrap

```mermaid
sequenceDiagram
    participant M as main.go
    participant C as Config Loader
    participant O as Orchestrator
    participant A as API Server :8080
    participant W as Webhook Server :8081
    participant S as Cron Scheduler

    M->>C: LoadConfig(configs/config.yaml)
    M->>C: LoadTestDefinitions(tests/)
    C-->>M: []TestDefinition
    M->>M: Register adapters<br/>(Store, Trigger, Assertions, Receivers, Notifier)
    M->>O: NewOrchestrator(adapters)
    par Servers in parallel
        M->>A: Start HTTP API
        M->>W: Start Webhook Server
        M->>S: Start Cron Scheduler
    end
    S->>S: Register scheduled tests<br/>(cron.AddFunc per test.schedule)
    A-->>M: Ready on :8080
    W-->>M: Ready on :8081
```

---

### 2. Sync Execution (async: false)

```mermaid
sequenceDiagram
    participant Client
    participant API as API :8080
    participant O as Orchestrator
    participant Redis
    participant T as HTTP Trigger
    participant R as Receivers
    participant WN as Failure Webhook

    Client->>API: POST /run?id=welcome_email
    API->>O: RunTest(testDef)
    O->>Redis: Reserve recipients (lock email → run_id)
    
    loop Retry loop (attempts N)
        O->>R: startReceivers() — IMAP connect / Redis listener
        O->>T: trigger.Execute(runID)
        T->>T: Inject {{run_id}}, {{env.*}} in URL/headers/body
        T->>+External: POST https://api.service.com/users
        External-->>-T: 200 OK + JSON response
        T->>T: Extract JSON paths → variables
        T-->>O: extracted vars

        par Collect in parallel
            O->>R: IMAP.Collect() — poll mailbox 2s tick
            O->>R: Request.Collect() — poll Redis 1s tick
        end

        R-->>O: Message (fields map)
        O->>O: Inject variables into assertions
        O->>O: Run assertions (contains/equals/matches...)
        O->>R: stopReceivers()

        alt Assertions passed
            O-->>O: break loop
        else Assertions failed
            O->>O: wait retry.delay
        end
    end

    O->>Redis: Release recipients (unlock)
    
    alt Test failed after all attempts
        O->>WN: Notify on_failure webhook
    end

    O-->>API: TestResult {status, attempts, duration, receivers}
    API-->>Client: 200 OK + TestResult JSON
```

---

### 3. Async Execution (async: true)

```mermaid
sequenceDiagram
    participant Client
    participant API as API :8080
    participant O as Orchestrator
    participant Redis

    Client->>API: POST /run?id=manual_imap_test
    API->>O: RunTest(testDef) — goroutine
    API-->>Client: 202 Accepted {run_id, status: "running"}

    Note over O: Executes in background<br/>(same flow as sync)
    O->>O: execute() — trigger + receivers + assertions
    O->>Redis: Store result (TTL 5min)

    loop Client polling
        Client->>API: GET /results/{run_id}
        alt Result available
            API->>Redis: Get result
            Redis-->>API: TestResult
            API-->>Client: 200 OK + TestResult
        else Still executing
            API-->>Client: 200 OK {status: "running"}
        end
    end
```

---

### 4. Retry Flow

```mermaid
flowchart TD
    A([Start execute]) --> B[Attempt #N]
    B --> C[startReceivers]
    C --> D[trigger.Execute]
    D --> E[collectAll — parallel]
    E --> F[Run assertions]
    F --> G{Did all pass?}
    G -- Yes --> H[stopReceivers]
    H --> I[Release recipients]
    I --> J([TestResult: passed])
    G -- No --> K[stopReceivers]
    K --> L{Any attempts left?}
    L -- Yes --> M[Wait retry.delay]
    M --> B
    L -- No --> N[Release recipients]
    N --> O[Notify on_failure webhook]
    O --> P([TestResult: failed])
```

---

### 5. IMAP Receiver

```mermaid
sequenceDiagram
    participant O as Orchestrator
    participant IR as IMAP Receiver
    participant IC as IMAP Client
    participant Server as IMAP Server
    participant P as Parser

    O->>IR: Start(ctx, runID)
    IR->>IC: Connect(host, port, tls)
    IC->>Server: TLS Dial + LOGIN
    Server-->>IC: OK authenticated
    IC-->>IR: connected

    Note over O: trigger.Execute() fires the email

    loop Poll every 2 seconds
        O->>IR: Collect(ctx)
        IR->>IC: SearchByRunID(ctx, runID)
        IC->>Server: SEARCH SUBJECT runID
        alt Email found in subject
            Server-->>IC: [uid1]
        else Not found
            IC->>Server: SEARCH BODY runID
            Server-->>IC: [uid1] or []
        end
        
        alt Email found
            IC->>Server: FETCH uid1 (BODY[])
            Server-->>IC: raw email bytes
            IC->>P: ParseMessage(runID, raw)
            P->>P: Parse MIME structure
            P->>P: Extract subject, from, to, date, body, html_body
            P-->>IC: domain.Message
            IC-->>IR: Message
            IR-->>O: Message
        else Timeout
            IR-->>O: error timeout
        end
    end

    O->>IR: Stop()
    IR->>IC: Disconnect() — IMAP LOGOUT
```

---

### 6. Request Receiver (Webhook)

```mermaid
sequenceDiagram
    participant T as HTTP Trigger
    participant Ext as External Service
    participant WS as Webhook Server :8081
    participant Ex as Extractor (Twilio/Meta)
    participant Redis
    participant RR as Request Receiver
    participant O as Orchestrator

    O->>RR: Start(ctx, runID)
    Note over RR: Ready to listen Redis

    T->>Ext: POST trigger (with run_id in payload)
    Ext->>WS: POST /webhook/twilio?token=jwt (callback)
    WS->>WS: Validate JWT
    WS->>Ex: Extract(*http.Request)
    Ex->>Ex: Parse form/JSON payload
    Ex->>Ex: Flatten fields → {from, to, body, ...}
    Ex-->>WS: domain.Message
    WS->>Redis: Deposit(runID, message, TTL 5min)

    loop Poll Redis every 1 second
        RR->>Redis: Claim(runID)
        alt Message available
            Redis-->>RR: Message
            RR-->>O: Message
        else No message yet
            Redis-->>RR: nil
        end
    end
```

---

### 7. Hexagonal Architecture

```mermaid
graph TB
    subgraph Domain["Domain (core)"]
        DM[TestDefinition<br/>TestResult<br/>Message<br/>ReceiverResult]
        ERR[Domain Errors]
    end

    subgraph Ports["Ports (interfaces)"]
        PT[Trigger]
        PR[Receiver]
        PA[Assertion]
        PS[Store]
        PN[Notifier]
        PE[Extractor]
        PI[IMAPClient]
    end

    subgraph Primary["Adapters — Primary (inbound)"]
        HTTP[HTTP API<br/>:8080<br/>/run /results /health]
        WH[Webhook Server<br/>:8081<br/>/webhook/twilio<br/>/webhook/meta]
        CR[Cron Scheduler]
    end

    subgraph Secondary["Adapters — Secondary (outbound)"]
        HTT[HTTP Trigger]
        IMAP[IMAP Receiver]
        REQ[Request Receiver]
        ASS[Assertions<br/>contains/equals/matches<br/>present/not_contains]
        RDS[Redis Store]
        NTF[Webhook Notifier]
    end

    subgraph Orch["Orchestrator (services)"]
        ORC[orchestrator.go<br/>execute / retry / collect / assert]
    end

    HTTP --> ORC
    WH --> RDS
    CR --> ORC
    ORC --> PT --> HTT
    ORC --> PR --> IMAP
    ORC --> PR --> REQ
    ORC --> PA --> ASS
    ORC --> PS --> RDS
    ORC --> PN --> NTF
    IMAP --> PI
    WH --> PE
    REQ --> PS
    ORC --> DM
```

---

## API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/health` | No | Liveness check |
| `POST` | `/run?id={test_id}` | Bearer token | Execute test |
| `GET` | `/results` | Bearer token | Last 100 results |
| `GET` | `/results/{run_id}` | Bearer token | Poll async result |
| `GET` | `/swagger/` | Bearer token | Interactive docs |
| `POST` | `/webhook/{provider}` | JWT query param | Receive webhook |

---

## Interesting Things

### Correlation without coupling via `{{run_id}}`
Each execution generates a unique UUID injected into the trigger payload. The system receives messages from the outside world and searches for that UUID — without the service under test having to do anything special. It only needs to propagate the `run_id` in the outgoing message (email, webhook, SMS).

### Reserved recipients with Redis lock
If two tests use the same receiver email, Redis serializes them. No race conditions between concurrent runs on the same IMAP mailbox.

### `options` map per receiver
Connection configuration (host, port, credentials) lives in the YAML, not in `configs/config.yaml`. The same receiver type can connect to different accounts per test. Adding a new receiver doesn't require global config changes.

### Extractor pattern
Webhook parsing (Twilio, Meta, etc.) is separated from the receiver. Adding a new provider = create a new `Extractor` + register it. The `Request Receiver` doesn't change.

### Async + Redis TTL = polling without DB
`async: true` returns 202 immediately. The result is stored in Redis with a 5-minute TTL. Stateless polling — no persistent results table.

### Add a new receiver in 5 steps
See `CONTRIBUTING.md`:
1. Create `internal/adapters/secondary/receiver/{channel}/receiver.go`
2. Implement `ports.Receiver` interface (Start, Collect, Stop)
3. Add infrastructure config in `configs/config.yaml` if applicable
4. Register in `cmd/server/main.go`
5. Use `type: {channel}` in YAML

### Configurable Auth
`auth.enabled: false` in config disables authentication — useful locally. The webhook server uses JWT via query param (`?token=...`) compatible with Twilio and Meta which don't support custom headers in callbacks.

### Autogenerated Swagger
`GET /swagger/` exposes interactive docs generated from annotations in the code. No need to maintain a manual spec.
