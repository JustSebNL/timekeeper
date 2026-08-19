# Time Keeper API

Base URL: `http://127.0.0.1:1618/api/v1`

Time Keeper is a local, SQLite-authoritative HTTP/JSON API. It accepts only explicit numeric loopback listeners (`127.0.0.1` or `::1`). It has no authentication and must not be exposed beyond the local user boundary.

Every `POST` mutation requires `Content-Type: application/json`. Unknown request fields and non-JSON media types are rejected. Health remains exactly:

```json
{"status":"ok"}
```

## Implemented endpoints

```text
GET  /health
GET  /api/v1/pulse
GET  /api/v1/guardian/status
GET  /api/v1/projects
POST /api/v1/projects
POST /api/v1/projects/{projectID}/metadata
POST /api/v1/projects/{projectID}/status
GET  /api/v1/projects/{projectID}/execution-tree
GET  /api/v1/projects/{projectID}/operational-summary
GET  /api/v1/projects/{projectID}/attention
GET  /api/v1/projects/{projectID}/events
GET  /api/v1/projects/{projectID}/notes
POST /api/v1/projects/{projectID}/notes
GET  /api/v1/projects/{projectID}/export
GET  /api/v1/projects/{projectID}/categories
POST /api/v1/projects/{projectID}/categories
  Optional body field: parent_category_id (a Category in the same Project).
POST /api/v1/categories/{categoryID}/metadata
POST /api/v1/categories/{categoryID}/status
GET  /api/v1/projects/{projectID}/tasks
POST /api/v1/projects/{projectID}/tasks
POST /api/v1/projects/{projectID}/sprints/claim-next
POST /api/v1/tasks/{taskID}/metadata
POST /api/v1/tasks/{taskID}/status
GET  /api/v1/tasks/{taskID}/subtasks
POST /api/v1/tasks/{taskID}/subtasks
GET  /api/v1/tasks/{taskID}/sprints
POST /api/v1/tasks/{taskID}/sprints
POST /api/v1/subtasks/{subtaskID}/status
GET  /api/v1/subtasks/{subtaskID}/sprints
POST /api/v1/subtasks/{subtaskID}/sprints
GET  /api/v1/sprints/{sprintID}/time-entries
GET  /api/v1/sprints/{sprintID}/extensions
POST /api/v1/sprints/{sprintID}/extensions
POST /api/v1/sprints/{sprintID}/start
POST /api/v1/sprints/{sprintID}/hold
POST /api/v1/sprints/{sprintID}/resume
POST /api/v1/sprints/{sprintID}/complete
POST /api/v1/sprints/{sprintID}/hold-reason
POST /api/v1/sprints/{sprintID}/cancel
GET  /api/v1/sprints/{sprintID}/retrieval-attempts
POST /api/v1/sprints/{sprintID}/retrieval-attempts
GET  /api/v1/llm-pipelines
POST /api/v1/llm-pipelines
GET  /api/v1/projects/{projectID}/planning-drafts
POST /api/v1/projects/{projectID}/planning-drafts
POST /api/v1/projects/{projectID}/planning-drafts/generate
POST /api/v1/projects/{projectID}/planning-drafts/{draftID}/apply
```

## Runnable Sprint queue

`POST /api/v1/projects/{projectID}/sprints/claim-next` accepts `{}` and atomically starts the oldest Open Sprint whose Project, Category, Task, and optional Subtask are all Open. It returns the claimed Sprint with `status: "Active"`. It returns `409 no_runnable_sprint` when nothing is eligible. This is deliberate agent pickup, not an implicit scheduler.

When a direct Task-owned Sprint completes and it was that Task's final direct Sprint (and the Task has no Subtasks), Time Keeper completes the parent Task and records the event. A Task with remaining queued Sprints remains Open.

A Task or Subtask may own one to four Sprints. New Sprints require every owning Project, Category, Task, and optional Subtask to be `Open`; Time Keeper rejects a new child under a terminal or held parent rather than silently reopening parent work. A Task also cannot be manually marked `Completed` while it still owns a non-terminal Sprint or Subtask.

An Open Sprint may transition directly to `On Hold` for any real blocker without creating a work interval. This is the correct state when you run out of road and other items must catch up first, a user decided otherwise, you are just waiting for input, or any external dependency: it remains visible, consumes no active capacity, and does not invent active time.

## Bounded retrieval attempts

`POST /api/v1/sprints/{sprintID}/retrieval-attempts` records immutable evidence that one retrieval cycle did not yield material progress:

```json
{"reason":"Provider returned no usable material after the bounded retrieval."}
```

A Sprint accepts exactly four attempts. The fourth atomically changes it to `TimedOut`, closes any active or held interval honestly, and records both attempt and timeout events. `TimedOut` remains visible and is not runnable; a fifth attempt is rejected. `GET /api/v1/sprints/{sprintID}/retrieval-attempts` returns the complete evidence history.

## Attention beyond Pulse

`GET /api/v1/projects/{projectID}/attention` is a read-only decision queue for work that Pulse deliberately does not report: every `On Hold` Sprint with its recorded broad reason, every dormant `TimedOut` Sprint, and any legacy Open Sprint made non-runnable by a non-Open parent. It never starts, resumes, or changes work.

`On Hold` has deliberately broad meaning: you can run out of road and other items must catch up first, a user decided otherwise, you are just waiting for input, or any other real blocker. An Open Sprint can move directly to `On Hold` only with a reason; that does not create active work time. `cancel` likewise requires a reason and leaves the Sprint in durable `Cancelled` history rather than deleting it.

## Pulse

`GET /api/v1/pulse` remains Time Keeper's local, read-only Sprint attention surface. It returns Active Sprints whose actual active work has exceeded their declared plan:

```text
estimate + buffer + approved extensions = planned duration
recorded active work + current active interval = active duration
active duration - planned duration = overdue duration
```

It never writes a database record, creates a Project event, schedules a background job, sends a message, or contacts a delivery service. An agent may poll it or schedule its own local check. `recommended_next_pulse_seconds` is a suggestion, not an automatic timer.

```json
{
  "format":"timekeeper-pulse/v1",
  "generated_at":"2026-08-12T10:00:00Z",
  "recommended_next_pulse_seconds":60,
  "attention":[{
    "kind":"sprint_overdue",
    "project_id":"P-10000",
    "sprint_id":"SP-10004",
    "item_address":"P-10000/C-10001/T-10002/SP-10004",
    "name":"Finish migration",
    "status":"Active",
    "planned_duration_seconds":1800,
    "active_duration_seconds":1920,
    "overdue_duration_seconds":120
  }]
}
```

Open, held, completed, and cancelled Sprints are not reported as overdue: Time Keeper has no due date for them. An empty `attention` array means no currently Active Sprint has exceeded its own declared plan.

## Pulse Guardian: durable attention recovery

A dashboard/API poll cannot get the attention of an agent whose work loop is stuck. `Pulse Guardian` is therefore a separate local worker. It is a required backbone service and runs by default every 5 minutes; the cadence can be overridden with `-pulse-guardian-interval`, or disabled explicitly by passing an empty interval:

```text
timekeeper -pulse-guardian-interval 1s
```

`GET /api/v1/guardian/status` reports whether this server process evaluates Guardian leases and its interval. It is runtime configuration only; a running Guardian still needs an agent to register a local recovery callback.

An agent opts in by renewing an explicit material-progress lease:

```http
POST /api/v1/agents/agent%3Aworker-7/progress
Content-Type: application/json

{
  "active_sprint_id":"SP-10004",
  "lease_duration_seconds":45,
  "guardian_url":"http://127.0.0.1:19090/pulse"
}
```

`guardian_url` is deliberately restricted to plain `http`, an explicit numeric loopback address (`127.0.0.1` or `::1`), a numeric nonzero port, and no userinfo, query, or fragment. It is not a general webhook feature. The lease is a progress assertion, not a TCP health check: the agent must renew it after material work. A later request that omits `active_sprint_id` and `guardian_url` preserves their existing values; send explicit empty strings to clear either value.

When `now - last_progress_at` is strictly greater than `lease_duration_seconds`, Time Keeper creates one durable `Pending` `agent_unresponsive` nudge for that agent. A durable pending nudge is never duplicated. On every Guardian tick, Time Keeper retries the registered local Guardian until it explicitly accepts the signal, and records `delivery_attempts`, `last_delivery_at`, and the first confirmed `delivered_at`. A confirmed callback is not sent again while that nudge remains pending acknowledgement. A callback is counted as accepted only when it returns a 2xx status and this exact response header:

```text
X-Timekeeper-Pulse-Accepted: v1
```

The callback body is versioned JSON:

```json
{
  "format":"timekeeper-pulse-guardian/v1",
  "action":"recover_attention",
  "nudge":{
    "nudge_id":41,
    "agent_id":"agent:worker-7",
    "active_sprint_id":"SP-10004",
    "kind":"agent_unresponsive",
    "status":"Pending",
    "detected_after_seconds":21,
    "delivery_attempts":0,
    "created_at":"2026-08-12T10:00:21Z"
  }
}
```

The independent callback process owns the real recovery action: notify, interrupt, stop, restart, or replace the watched agent according to its own explicit local policy. Time Keeper does **not** execute arbitrary commands, kill processes, start processes, message external services, or pretend delivery proves recovery. After recovery is observed, the owning agent acknowledges the durable nudge:

```http
POST /api/v1/agents/agent%3Aworker-7/nudges/41/ack
Content-Type: application/json

{}
```

Acknowledgement is owner-bound and idempotent. It changes the nudge to `Acknowledged` and renews that agent's lease. `GET /api/v1/agents/{agentID}/nudges` returns only currently pending recovery work; `GET /api/v1/agents/{agentID}/nudges/history` retains the durable pending-and-acknowledged audit trail.

Implemented Guardian endpoints:

```text
POST /api/v1/agents/{agentID}/progress
GET  /api/v1/agents/{agentID}/nudges
GET  /api/v1/agents/{agentID}/nudges/history
POST /api/v1/agents/{agentID}/nudges/{nudgeID}/ack
```

The original `GET /api/v1/pulse` contract stays read-only. Guardian state is separate durable agent-attention evidence; it does not append Project events or alter Sprint accounting.

## Project context

A Project's durable goal and description can be updated without changing its identity or workflow status:

```http
POST /api/v1/projects/P-10000/metadata
Content-Type: application/json

{"goal":"Ship a local tracker","description":"Durable execution context."}
```

Goal is bounded to 1,000 characters and description to 10,000. A material change atomically appends immutable `project_metadata_updated` activity; an identical update does not add event noise.

## Status workflows

Projects and Tasks accept these explicit values:

```text
Open | On Hold | Completed | Cancelled
```

Example:

```http
POST /api/v1/projects/P-10000/status
Content-Type: application/json

{"status":"On Hold"}
```

Non-no-op changes append immutable Project activity. Sprint status follows its separate lifecycle:

```text
Open → Active → On Hold → Active → Completed
```

Illegal Sprint actions return HTTP `409` with `invalid_transition` and the allowed actions.

## Sprint extensions

A Sprint extension is immutable evidence of justified additional planned time. It does not overwrite the original estimate or an existing extension.

```http
POST /api/v1/sprints/SP-10004/extensions
Content-Type: application/json

{"duration_seconds":600,"reason":"Migration uncovered an additional compatibility case."}
```

`duration_seconds` must be greater than zero and no more than ten years in seconds. `reason` is required after trimming and is bounded to 10,000 characters. The endpoint returns HTTP `201` and the created extension record. `GET /api/v1/sprints/{sprintID}/extensions` returns the immutable extension history in chronological order as:

```json
{"items":[{"extension_id":2,"sprint_id":"SP-10004","duration_seconds":600,"reason":"Migration uncovered an additional compatibility case.","created_at":"2026-08-11T00:00:00Z"}]}
```

## Local LLM planning (review-only)

A local pipeline is configuration for either Ollama or an OpenAI-compatible local server (such as vLLM or llama.cpp). Only plain HTTP numeric-loopback base URLs are accepted; remote hosts, `localhost`, HTTPS, paths, credentials, query strings, and fragments are rejected.

```http
POST /api/v1/llm-pipelines
Content-Type: application/json

{
  "name":"Planner",
  "provider":"ollama",
  "base_url":"http://127.0.0.1:11434",
  "model":"qwen3:4b"
}
```

Generate a draft for a Project:

```http
POST /api/v1/projects/P-10000/planning-drafts/generate
Content-Type: application/json

{"pipeline_id":1}
```

Generation passes bounded authoritative Project context to the selected local model. Model output must strictly validate as `timekeeper-planning-draft/v1`; it is stored as an immutable `Review` artifact with its exact raw JSON.

Apply requires a separate, deliberate request:

```http
POST /api/v1/projects/P-10000/planning-drafts/9/apply
Content-Type: application/json

{}
```

Apply revalidates the persisted artifact and atomically materializes its Categories, Tasks, Subtasks, and Sprints. A draft is then marked `Applied` and cannot be applied again. No model response directly mutates hierarchy.

## Project export and SQLite backup

`GET /api/v1/projects/{projectID}/export` is a versioned Project-scoped JSON export. It is not a complete database recovery snapshot.

Use server backup mode for a recoverable SQLite snapshot:

```text
timekeeper -db timekeeper.db -backup-to timekeeper-backup.db
```

The destination must not exist. Backups use `VACUUM INTO`, request owner-only mode on Unix-like filesystems, and remain unencrypted.
