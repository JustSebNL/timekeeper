# API Bug Reporter

Status: implemented and verified

## Purpose

The first reporter version is specifically for **TimeKeeper alpha-testing defects**. It preserves the local-only boundary and routes defects to the explicitly selected owning Project; it does not silently reassign consumer-project defects.

The endpoint is implemented in `internal/api/bug_reports.go`, with SQLite persistence in `internal/store/bug_reports.go` and model definitions in `internal/model/bug_report.go`.

## Current API boundary

TimeKeeper currently serves a local SQLite-authoritative API on an explicit numeric loopback listener. It has no authentication and must not be exposed beyond the local user boundary. The first reporter version must preserve that boundary. A future caller-authentication scheme may be added separately; a payload field such as `agent_id` is attribution, not authentication.

All mutations use `Content-Type: application/json`, except the bug-report endpoint, which also accepts bounded `text/markdown`. Unknown fields are rejected and input sizes are bounded.

## Endpoint

```text
POST /api/v1/bug-reports
```

The report selects the owning Project explicitly. A report about TimeKeeper itself uses the TimeKeeper Project and its defect category (`P-10053` / `C-10139` in the current installation). A report discovered while working on another project remains owned by that project's Project record unless it is specifically a TimeKeeper defect.

### Request shape

```json
{
  "project_id": "P-10053",
  "category_id": "C-10139",
  "task_id": "T-10178",
  "sprint_id": "SP-10179",
  "title": "Sprint creation returns HTTP 400 for an open task",
  "severity": "P2",
  "expected": "A valid open Task accepts a new Sprint.",
  "actual": "The API returns HTTP 400 without actionable validation details.",
  "reproduction_steps": [
    "Use an open Task in an open Project and Category.",
    "POST a valid Task-owned Sprint request.",
    "Observe HTTP 400."
  ],
  "environment": {
    "os": "Windows",
    "component": "TimeKeeper API",
    "version": "local checkout"
  },
  "source": {
    "agent_id": "codex",
    "session_id": "session-reference",
    "command": "tk sp new task ..."
  },
  "evidence": [
    {"type": "command_output", "ref": "local-run-reference"},
    {"type": "message", "ref": "M-112"}
  ],
  "dedupe_key": "sha256:...",
  "idempotency_key": "report-attempt-uuid",
  "redaction_status": "verified"
}
```

### Markdown input

The same endpoint accepts `Content-Type: text/markdown` with bounded frontmatter and sections. Structured fields remain mandatory; Markdown is only an alternate input format.

```markdown
---
project_id: P-10053
severity: P2
idempotency_key: alpha-report-001
redaction_status: verified
source.agent_id: codex
source.session_id: local-session
source.command: tk sp new task ...
---
# Sprint creation returns HTTP 400

## Expected
A valid open Task accepts a new Sprint.

## Actual
The API returns HTTP 400 without actionable validation details.

## Reproduction
- Use an open Task in an open Project and Category.
- POST a valid Task-owned Sprint request.
- Observe HTTP 400.

## Environment
- component: TimeKeeper API
```

The parser rejects unknown frontmatter fields, unsupported sensitive markers, missing required sections, and bodies above the normal 1 MiB mutation limit. It stores the parsed structured record, not the raw Markdown dump.

Required fields are `project_id`, `title`, `severity`, `expected`, `actual`, `reproduction_steps`, `source`, `idempotency_key`, and `redaction_status`. `category_id`, `task_id`, and `sprint_id` are optional references but must resolve within the selected Project when present.

Suggested severity values are `P0`, `P1`, `P2`, and `P3`. The route should also accept a small controlled `kind` field in a later schema revision if reports need to distinguish bugs, data defects, security issues, and UX problems.

## Response

Successful creation returns `201 Created`; idempotent retries return `200 OK` with `duplicate: true`. The first-class bug record is the durable pointer.

```json
{
  "report_id": "BR-10001",
  "status": "New",
  "project_id": "P-10053",
  "task_id": "T-10178",
  "duplicate": false
}
```

The implementation uses a first-class `bug_reports` record and immutable Project events. It does not create a Task or Sprint automatically; implementation work remains explicitly planned.

Retries with the same `idempotency_key` must return the original result without creating a second report. A matching `dedupe_key` should return the existing report as `duplicate: true` while retaining the new evidence as a related observation or explicitly rejecting it with a useful conflict response.

## Status transitions

```text
POST /api/v1/bug-reports/{reportID}/status
```

Request body:

```json
{"project_id":"P-10053","status":"Confirmed"}
```

Allowed statuses are `New`, `Confirmed`, `In Progress`, `Fixed`, `Verified`, `Duplicate`, and `Rejected`. Each transition is recorded as an immutable Project event.

## Validation and privacy

- Keep the endpoint loopback-only under the current API boundary.
- Reject credentials, cookies, bearer tokens, private keys, passwords, connection strings, raw prompt bodies, full transcripts, and arbitrary tool-output dumps.
- Validate and bound every string, array, evidence item, and environment field.
- Treat evidence references as pointers; do not ingest unbounded files through this endpoint.
- Redaction must be checked before persistence, with `redaction_status` required rather than assumed.
- Do not use opaque identifiers as authorization. Access control, ownership checks, and audit events remain explicit responsibilities.
- Record immutable creation and status-transition events so a report can be traced from discovery to verification.

## Triage lifecycle

The report should support a small lifecycle independent of Sprint accounting:

```text
New -> Confirmed -> In Progress -> Fixed -> Verified
             \-> Duplicate
             \-> Rejected
```

Implementation work should still use normal Project, Category, Task, and Sprint records. A report links to the work; it does not silently start a Sprint or alter task accounting.

## TimeKeeper defects

When a report identifies a TimeKeeper defect, it should route to the TimeKeeper Project rather than the consumer Project. The current defect category is `C-10139` (`Alpha defects found by task harness`). The reporter should preserve the original command, response status, exact reproduction context, and any relevant `M-` reference without copying sensitive output.

## Acceptance criteria

1. A valid report creates one durable, queryable record and returns stable IDs.
2. Invalid project/task/category references are rejected with field-level errors.
3. Repeated idempotency keys do not create duplicate records.
4. Redaction and size limits are enforced before persistence.
5. Creation and status changes appear in the Project activity history.
6. The dashboard and `tk` CLI can discover the report or its linked Task.
7. API, validation, persistence, idempotency, redaction, and authorization-boundary tests cover the route.
8. `API.md` documents the final route only after implementation and verification.
