---
name: e2e-test-writer
description: |
  Guides creation of YAML test files for this e2e-framework project.
  Produces correctly structured, idiomatic YAML tests following existing patterns
  in tests. Use when creating new tests, adding test steps, or validating
  test YAML against the framework schema.
triggers:
  - create a test
  - write a test
  - add e2e test
  - new yaml test
  - new test
  - test this endpoint
  - test this flow
  - add a test case
  - write yaml test
---

# E2E Test Writer

You are authoring a YAML test for this Go-based e2e-framework. Tests are HTTP-driven, sequential, and support variable extraction between steps.

## Step 1 — Gather Requirements

Before writing YAML, collect (ask if not provided):

1. **Suite / feature name** — determines directory (`tests/<suite>/`) and file prefix
2. **Test number** — next sequential number in that suite (check `tests/<suite>/` for existing files)
3. **Test description** — one sentence, what behavior is verified
4. **Trigger steps** — for each step:
   - HTTP method + URL
   - Expected HTTP status code (required unless truly any code is acceptable)
   - Request headers (Authorization, Content-Type, etc.)
   - Request body (POST/PUT/PATCH only)
   - Fields to extract from response (for use in later steps)
   - Response assertions (field, type, expected value)
5. **Receivers** — does any step trigger an async notification? (email via IMAP, webhook via `request` type)
6. **Schedule** — does this run on a cron? If yes, add `on_failure.calls`
7. **Retry** — default `attempts: 3, delay: 15s` unless user specifies otherwise

---

## Step 2 — Naming Convention

- **File**: `tests/<suite>/<suite>_NN_<snake_case_description>.yaml`
- **ID**: same as filename without `.yaml` (e.g., `test_03_create_user`)
- IDs must be unique across ALL yaml files in `tests/` (recursive)

---

## Step 3 — Generate YAML

Use this canonical structure. Do NOT add fields not listed here unless they map to the `TestDefinition` schema.

```yaml
version: "1"
id: <suite>_NN_<description>
description: "<what this test verifies>"
enabled: true
async: false              # true only for long-running async flows

retry:
  enabled: true
  attempts: 3
  delay: 15s

triggers:
  - method: POST           # POST | GET | PUT | DELETE | PATCH
    url: "https://host/path/{{extracted_var}}"
    timeout: 10s
    expected_status: 202   # omit ONLY if any status is valid
    headers:
      Authorization: "Bearer {{env.TOKEN_ENV_VAR}}"
      Content-Type: "application/json"
    body:                  # omit for GET/DELETE
      field: value
      source_id: "{{run_id}}"   # use run_id for idempotency keys
    extract:               # capture response JSON fields into variables
      my_var: "json.path"  # available as {{my_var}} in all subsequent triggers
    response_assertions:
      - type: equals
        field: "json.field"
        value: "EXPECTED_VALUE"
    receivers:             # include ONLY when trigger causes async notification
      - type: imap         # imap | request
        timeout: 60s
        recipient: ""      # optional: email/phone to filter by
        options:
          host: "{{env.IMAP_HOST}}"
          port: "{{env.IMAP_PORT}}"
          username: "{{env.IMAP_USERNAME}}"
          password: "{{env.IMAP_PASSWORD}}"
          mailbox: INBOX
          tls: "true"
        assertions:
          - type: contains
            field: subject
            value: "{{run_id}}"
    wait_for_receivers: true   # required when receivers is present
```

---

## Assertion Types

| type | when to use |
|---|---|
| `equals` | exact string/value match |
| `contains` | substring present in field value |
| `not_contains` | substring must NOT be present |
| `matches` | field matches regex pattern in `value` |
| `present` | field exists (omit `value`) |

**Fields for response_assertions**: dot-notation JSON path from response body root (e.g., `"code"`, `"data.id"`, `"errors.0.message"`)

**Fields for imap receiver assertions**: `subject`, `body`, `from`, `to`

**Fields for request receiver assertions**: `from`, `body`, `headers.<name>`

---

## Template Variables

| variable | source | notes |
|---|---|---|
| `{{run_id}}` | auto-generated per run | use for `source_id`, unique keys, traceability |
| `{{test_id}}` | test definition `id` field | |
| `{{env.VAR_NAME}}` | OS environment variable | credentials, tokens, hostnames |
| `{{extracted_var}}` | prior trigger `extract:` key | only available after the step that defines it; also valid in `on_failure.calls` |
| `{{error}}` | failure context | only valid in `on_failure.calls` |

---

## on_failure Block

Add when: test has a `schedule:` field OR it covers a critical production flow.

Calls run sequentially, asynchronously and never block the failure result. If a call references a variable that was never extracted, that call is skipped and a warning is logged — the rest still run.

```yaml
on_failure:
  calls:
    - method: POST
      url: "{{env.ALERT_WEBHOOK_URL}}"
      timeout: 10s
      headers:
        Content-Type: application/json
      body:
        test_id: "{{test_id}}"
        run_id: "{{run_id}}"
        error: "{{error}}"
        transaction_id: "{{transaction_id}}"
    - method: POST
      url: "https://alerts.company.com/tickets"
      expected_status: 201
      body:
        test_id: "{{test_id}}"
        error: "{{error}}"
```

Supported fields per call: `method` (default `POST`), `url`, `timeout` (default `15s`), `delay_before`, `headers`, `body` (JSON; `Content-Type` auto-set) and `expected_status`.

**Rules:**
- Any `{{var}}` in a call must be defined in a PRIOR trigger's `extract:` (or be a built-in: `run_id`, `test_id`, `error`). Otherwise the call is skipped.
- `{{env.VAR}}` works here too — resolved at config load time.

---

## Quality Checklist

Before finalizing, verify:

- [ ] `id` matches filename (without `.yaml`)
- [ ] `id` is unique — check existing files in `tests/` with `find tests/ -name "*.yaml" | xargs grep "^id:"`
- [ ] Every trigger that modifies state has `expected_status`
- [ ] Every trigger has `timeout`
- [ ] Variable in `{{var}}` is defined in a PRIOR trigger's `extract:` (not the same trigger)
- [ ] `wait_for_receivers: true` is set whenever `receivers:` is present
- [ ] Env vars are named consistently with existing tests (check `tests/` for conventions)
- [ ] `source_id: "{{run_id}}"` used wherever the API supports idempotency keys
- [ ] Credentials are `{{env.VAR}}` — never hardcoded

---

## File Placement

```
tests/
├── example_welcome_email.yaml    # standalone/example tests
├── crud_productos.yaml
└── user/                        # suite-specific subdirectory
    ├── user_01_create_user.yaml
    ├── user_02_get_created_user.yaml
    └── ...
```

Create `tests/<suite>/` if it doesn't exist. File must have `.yaml` extension — the loader recurses all subdirectories.

---

## Complete Example (two-step create + verify)

```yaml
version: "1"
id: user_01_create_user
description: "Create a new user"
enabled: true
async: false

retry:
  enabled: true
  attempts: 3
  delay: 15s

triggers:
  - method: POST
    url: "https://api.example.com/v1/user"
    timeout: 10s
    expected_status: 202
    headers:
      Authorization: "Bearer {{env.API_TOKEN}}"
      Content-Type: "application/json"
    body:
      user_id: "74ff0601-eb9f-4756-83a5-65cca337911c"
      source_id: "{{run_id}}"
      template: "welcome_push"
    response_assertions:
      - type: equals
        field: "code"
        value: "NOTIFICATION_CREATED"
    extract:
      notification_id: "id"

  - method: GET
    url: "https://api.example.com/v1/notifications/{{notification_id}}"
    timeout: 10s
    expected_status: 200
    headers:
      Authorization: "Bearer {{env.API_TOKEN}}"
    response_assertions:
      - type: equals
        field: "status"
        value: "pending"
```
