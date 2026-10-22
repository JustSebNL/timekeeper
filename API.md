# Time Keeper API

Base URL: `http://timekeeper.local/api/v1`

Agents should use this friendly hostname as the local API entry point or omit `--url` to let the CLI select its default. Explicit `--url` and `TIMEKEEPER_URL` values are used verbatim.

Time Keeper is a local, SQLite-authoritative HTTP/JSON API. It has no authentication and must not be exposed beyond the local user boundary.

JSON mutations use `Content-Type: application/json`, and structured JSON handlers reject unknown fields. The bug-report endpoint also accepts bounded `text/markdown` input. Unsupported media types are rejected. Health remains exactly:

```json
{"status":"ok"}
```

## Core project and Sprint endpoints

```text
GET  /health
GET  /api/v1/pulse
GET  /api/v1/guardian/status
GET  /api/v1/projects
GET  /api/v1/project-deadlines
GET  /api/v1/search?q=<query>&scope=projects&scope=tasks&scope=messages&limit=20
POST /api/v1/projects
POST /api/v1/projects/{projectID}/metadata
POST /api/v1/projects/{projectID}/status
PATCH /api/v1/projects/{projectID}/schedule
POST /api/v1/projects/{projectID}/phase
GET  /api/v1/projects/{projectID}/milestones
POST /api/v1/projects/{projectID}/milestones
PATCH /api/v1/projects/{projectID}/milestones/{milestoneID}
POST /api/v1/projects/{projectID}/milestones/{milestoneID}/status
GET  /api/v1/projects/{projectID}/execution-tree
GET  /api/v1/projects/{projectID}/routing/candidates
POST /api/v1/projects/{projectID}/routing/category
POST /api/v1/projects/{projectID}/routing/decide
POST /api/v1/projects/{projectID}/routing/apply
GET  /api/v1/projects/{projectID}/operational-summary
GET  /api/v1/projects/{projectID}/usage-summary
GET  /api/v1/projects/{projectID}/usage-sessions/{sessionID}/snapshots
POST /api/v1/projects/{projectID}/usage-sessions/{sessionID}/snapshots
GET  /api/v1/projects/{projectID}/attention
GET  /api/v1/projects/{projectID}/events
GET  /api/v1/projects/{projectID}/notes
POST /api/v1/projects/{projectID}/notes
GET  /api/v1/projects/{projectID}/messages
POST /api/v1/projects/{projectID}/messages
POST /api/v1/projects/{projectID}/messages/checkpoint
GET  /api/v1/projects/{projectID}/messages/search
GET  /api/v1/projects/{projectID}/messages/{messageID}
PATCH /api/v1/projects/{projectID}/messages/{messageID}
GET  /api/v1/projects/{projectID}/messages/{messageID}/thread
GET  /api/v1/projects/{projectID}/messages/{messageID}/context
GET  /api/v1/projects/{projectID}/messages/{messageID}/revisions
POST /api/v1/projects/{projectID}/messages/{messageID}/pin
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

`GET /api/v1/projects/{projectID}/execution-tree` accepts optional `task_id`, `status`, `active=true`, `depth` (1–10), and `limit` (1–200) query filters. Task/status filters preserve their Category owner paths; `active=true` keeps only owners of Active Sprints. The response remains an `ExecutionTree` JSON object. When `limit` is set, `X-TimeKeeper-Execution-Tree-Total-Tasks`, `X-TimeKeeper-Execution-Tree-Shown-Tasks`, and `X-TimeKeeper-Execution-Tree-Truncated` report counts after other filters and before/after the stable prefix limit. Invalid filter values return HTTP 400. With no query filters, the existing response shape is unchanged.

`GET /api/v1/projects/{projectID}/export?task_id={taskID}` returns the Project and owning Category path for only that Task, plus `filtered_by.task_id`; it omits notes, events, milestones, usage, and operational summaries. A missing Task produces an empty filtered tree. The CLI `tk export <project-id> [file] --task <T-ID>` filters locally as well, so it also strips note/event bodies if talking to an older Core that ignores the query. Unfiltered Project export is unchanged.

`GET /api/v1/projects/{projectID}/routing/candidates` returns the bounded candidate set and stable state hash used by Core policy. ShadowBox must build both Jev passes from this exact snapshot and send `candidate_state_hash` with each evaluation request. `POST /api/v1/projects/{projectID}/routing/category` validates the first Category-only pass; `POST /api/v1/projects/{projectID}/routing/decide` validates the complete typed routing decision. Core re-reads current state and returns `409 routing_candidate_state_changed` if the supplied hash is stale. These three evaluation/read routes are read-only; they do not call Jev, persist a decision, or mutate the Project hierarchy. Connection capabilities advertise `routing.candidates.read`, `routing.category.evaluate`, and `routing.proposal.evaluate`.

`POST /api/v1/projects/{projectID}/routing/apply` is the Core-owned, confirmation-gated write boundary. It requires a matching `decision_id`, `candidate_state_hash`, `confirmed: true`, and an `Idempotency-Key`. Core validates the current snapshot and policy, then submits one execution batch; the queue worker revalidates the original snapshot transactionally before registering it. The initial supported actions are only `create_sprint` under an existing Task/Subtask and atomic `create_task_and_sprint` under an existing Category. Category creation, Sprint resumption, and other actions are rejected. Matching idempotent retries return the original queue/result; reusing the key for a changed payload conflicts. Connection capabilities advertise `routing.proposal.apply`. The ShadowBox CLI `--apply` path displays the exact Core decision and requires the operator to type `APPLY <decisionID>` before sending the confirmed request. The source route is not installed in the running service. Core API-key authentication and remote execution remain out of scope; do not expose remote Apply.

All message routes accept either the canonical `P-{number}` Project ID or that Project's exact alias. The server resolves aliases before applying project scoping, so `/api/v1/projects/hsam/messages` and `/api/v1/projects/P-10000/messages` address the same board. The CLI applies the same resolution before issuing the request.

## Project schedule and lifecycle

Project creation accepts optional date-only schedule fields. Dates use `YYYY-MM-DD`; `schedule_timezone` is an IANA timezone and defaults to `UTC`. The created Project starts in lifecycle phase `intake` with a seven-day warning window.

```json
{
  "name": "Product launch",
  "planned_start_date": "2026-10-15",
  "target_end_date": "2026-12-01",
  "schedule_timezone": "Europe/Amsterdam",
  "deadline_warning_days": 10
}
```

`PATCH /api/v1/projects/{projectID}/schedule` accepts any subset of `planned_start_date`, `target_end_date`, `schedule_timezone`, and `deadline_warning_days`. Omitted values are preserved; an empty string explicitly clears an optional date. Planned start must not be later than target end. The returned Project includes derived `deadline_state` and `days_to_deadline`; state is one of `unscheduled`, `on_track`, `due_soon`, `overdue`, `completed`, or `cancelled`.

`GET /api/v1/project-deadlines` returns the dashboard watch as `{ "items": [...] }`, including overdue and due-soon Projects and milestones. Items are overdue-first and include `project_id`, `entity_id`, `entity_type`, `target_date`, `deadline_state`, and `days_to_deadline`. Completed/cancelled items are excluded.

`POST /api/v1/projects/{projectID}/phase` accepts `{ "phase": "planning", "reason": "..." }`. The legal path is `intake → planning → execution → validation → closeout`; one-step replanning/backtracking is allowed with a reason. Entering `execution` sets `actual_started_at` once. Setting Project status to `Completed` sets `completed_at` and moves the lifecycle phase to `closeout`; `Cancelled` sets `cancelled_at`. Completing requires an evidence summary in `reason`; cancelling requires a reason. The existing statuses (`Open`, `On Hold`, `Completed`, `Cancelled`) remain distinct from lifecycle phase.

Milestones are dated deliverable checkpoints, optionally linked to a Task in the same Project:

```text
GET    /api/v1/projects/{projectID}/milestones
POST   /api/v1/projects/{projectID}/milestones
PATCH  /api/v1/projects/{projectID}/milestones/{milestoneID}
POST   /api/v1/projects/{projectID}/milestones/{milestoneID}/status
```

Create body: `{"name":"Beta acceptance","description":"Acceptance suite passes","target_date":"2026-11-15","linked_task_id":"T-123"}`. Status values are `Planned`, `In Progress`, `Completed`, and `Cancelled`. Cancelling or reopening a milestone requires a reason. Milestone/project schedule changes and lifecycle transitions create durable Project events; project export includes all milestones.

Agent CLI equivalents are `tk p schedule`, `tk p phase`, and `tk p milestones list|add|date|status`.

## Additional implemented endpoints

The local server also exposes the following service, agent, administrator, and optional integration routes. Optional providers can return a typed unavailable response when they are not configured.

```text
GET  /health
GET  /api/help
GET  /api/v1/version
GET  /api/v1/connection-info

POST /api/v1/agent-identities/resolve
GET  /api/v1/agent-profiles/{identity}
GET  /api/v1/agent-profiles/{identity}/view
GET  /api/v1/agents/work-states
GET  /api/v1/agents/{agentID}/work-state
POST /api/v1/agents/{agentID}/activity
GET  /api/v1/agents
POST /api/v1/agents/broadcast
GET  /api/v1/agents/{agentID}/progress
POST /api/v1/agents/{agentID}/progress
GET  /api/v1/agents/{agentID}/nudges
GET  /api/v1/agents/{agentID}/nudges/history
POST /api/v1/agents/{agentID}/nudges/{nudgeID}/ack

GET  /api/v1/admin/me
GET  /api/v1/admin/users
PATCH /api/v1/admin/users/{userID}
GET  /api/v1/admin/groups
POST /api/v1/admin/groups
PUT  /api/v1/admin/groups/{groupID}/members
GET  /api/v1/admin/permissions
GET  /api/v1/admin/audit

POST /api/v1/bug-reports
GET  /api/v1/projects/{projectID}/bug-reports
POST /api/v1/bug-reports/{reportID}/status
POST /api/v1/projects/{projectID}/alias
GET  /api/v1/projects/{projectID}/coding-outcomes
POST /api/v1/projects/{projectID}/coding-outcomes
POST /api/v1/sprints/{sprintID}/supervised-launches

GET  /api/v1/knowledge/status
POST /api/v1/knowledge/search
GET  /api/v1/knowledge/{id}
POST /api/v1/projects/{projectID}/messages/{messageID}/promote
GET  /api/v1/plugins
GET  /api/v1/plugins/{pluginID}
GET  /api/v1/plugins/{pluginID}/artifact
```

`GET /api/v1/connection-info` returns `service`, stable SQLite-backed `instance_id`, version/build fields, `api_version`, sorted capability names, and `friendly_urls`. Optional capabilities appear only when their backing service is configured; `friendly_urls` is always an array. Copies made by database backup/restore preserve the same instance ID; a newly initialized database receives a different one. This endpoint is discovery metadata, not remote authentication or failover authorization.

The administrator routes enforce local permission metadata, not authenticated identities. A caller-supplied `X-Agent-ID` is not proof of identity; see [Administrator and security groups](docs/administrator.md). FASE identity resolution is automatic in compatible CLI builds and is also attribution, not authentication; see [FASE identity](docs/fase-identity-design.md).

File-history endpoints are listed in [File history and editor API](#file-history-and-editor-api); they require explicit `-history-root` configuration. The plugin and knowledge routes are optional local provider surfaces; see [extensions](docs/extensions.md) and [the Memory Galaxy boundary](#memory-galaxy-provider-boundary).

## Project Treasure Chest

`tk tc` is a CLI-first, project-scoped Treasure Chest backed by dedicated Core-owned `project_gems` and `project_gem_revisions` tables in the existing SQLite database. A gem has its own `G-<id>` reference, name, content, category, optional subcategory, tags, source reference, pin state, soft-delete timestamp, and optional parent-gem reference. Adding a gem with a new category or subcategory persists those labels automatically; `categories` derives the project’s active category tree from stored gems. Child gems are attached through `parent_gem_id`, which the Store restricts to the same Project. Edits increment the gem revision and preserve a snapshot in `project_gem_revisions`.

`delete` is a reversible soft-delete: it sets `deleted_at`, hides the gem from default list/lookup results, and preserves its attachment lineage and revisions. Use `restore` to bring it back, or `include_deleted=true` for inspection. Core enforces limits of 200 characters for names, 120 for category/subcategory labels, 20,000 for content, 500 for tags, and 2,048 for source references. The Treasure Chest is not stored as project messages and does not create a second database.

```http
GET    /api/v1/projects/{projectID}/treasure-chest/gems?category=Research&limit=50
POST   /api/v1/projects/{projectID}/treasure-chest/gems
GET    /api/v1/projects/{projectID}/treasure-chest/gems/search?q=retry
GET    /api/v1/projects/{projectID}/treasure-chest/gems/{gemID}
PATCH  /api/v1/projects/{projectID}/treasure-chest/gems/{gemID}
GET    /api/v1/projects/{projectID}/treasure-chest/gems/{gemID}/revisions
POST   /api/v1/projects/{projectID}/treasure-chest/gems/{gemID}/pin
POST   /api/v1/projects/{projectID}/treasure-chest/gems/{gemID}/delete
POST   /api/v1/projects/{projectID}/treasure-chest/gems/{gemID}/restore
GET    /api/v1/projects/{projectID}/treasure-chest/categories
```

The gem list accepts `category`, `subcategory`, `parent_gem_id`, `pinned=true`, `include_deleted=true`, and `limit` (1–200; omitted/0 uses the default). The search endpoint additionally requires `q` and searches the dedicated gem name/content/category/subcategory/tags/source fields. Pin uses an explicit JSON body, `{"pinned":true|false}`; delete/restore are idempotent POST transitions.

```text
tk tc add P-10000 "Research" "Retry budget" "Monotonic deadlines reduce retry storms."
tk tc add P-10000 "Research" "Jitter window" --subcategory "Backoff" "Bound synchronized retries."
tk tc attach P-10000 G-42 "Counterexample" "A fixed delay can synchronize clients."
tk tc lookup P-10000 retry budget --category "Research"
tk tc list P-10000 --category "Research"
tk tc show P-10000 G-42
tk tc pin P-10000 G-42
tk tc delete P-10000 G-42
tk tc restore P-10000 G-42
tk tc revisions P-10000 G-42
```

## Trail of Thought Board

The message board is the durable project-scoped memory surface for short reasoning, discoveries, decisions, questions, answers, links, and lessons. A post body is limited to 1000 Unicode characters so it remains compact enough to reference from an agent context window without turning into a second transcript store.

Each created post returns a numeric `message_id` and compact `ref` such as `M-42`. The `M-42` reference is stable and can be stored in an agent's context instead of the full body. Replies use `parent_message_id`; thread reads walk from any post or reply to the root and return deterministic parent-before-child order.

```text
POST /api/v1/projects/{projectID}/messages
{"kind":"observation","author":"agent-7","body":"The queue stalls after Sprint promotion.","tags":"queue,debug"}

POST /api/v1/projects/{projectID}/messages
{"kind":"answer","body":"The save boundary throws before processSprint.","parent_message_id":42}

PATCH /api/v1/projects/{projectID}/messages/42
{"kind":"lesson","author":"agent-7","body":"Persist state before promoting a Sprint.","tags":"queue,lesson"}

POST /api/v1/projects/{projectID}/messages/checkpoint
{"author":"agent-7","entries":[{"kind":"decision","body":"Persist before dispatch.","tags":"queue"},{"kind":"lesson","body":"Keep only the root M-ref in active context.","tags":"handoff"}]}

POST /api/v1/projects/{projectID}/messages/42/pin
{"pinned":true}

GET /api/v1/projects/{projectID}/messages/42/thread
GET /api/v1/projects/{projectID}/messages/42/context?thread_limit=12&related_limit=8&revision_limit=4
GET /api/v1/projects/{projectID}/messages/42/revisions
GET /api/v1/projects/{projectID}/messages/search?q=queue%20Sprint
```

`GET /api/v1/search` is the read-only cross-Project search boundary for Projects, Tasks, and project messages. Repeat `scope=projects`, `scope=tasks`, or `scope=messages` to narrow results; omitting scope searches all three. Optional `project=P-ID` narrows the owner. `limit` defaults to 20 and is capped at 200. Message-only filters are `kind`, comma-separated `tags`, and inclusive `after`/`before` dates (ISO `YYYY-MM-DD`, interpreted in UTC, or exact RFC3339 timestamps). The response carries attributed `items`, exact pre-limit `total`, `limit`, and `has_more`.

Message filters also work on the legacy Project-scoped endpoint `GET /api/v1/projects/{projectID}/messages/search`. When `kind`, `tags`, `after`, or `before` is supplied, that endpoint adds `total`, `limit`, and `has_more`; unfiltered calls retain the original `{items, query}` envelope. The terse form is `tk msg <P-ID> search <query...> [--limit N] [--kind K] [--tags T1,T2] [--after DATE] [--before DATE]`.

Ranking is deterministic: Project title/context matches precede Task title/context matches, which precede messages. Within Projects/Tasks, a match where every query token occurs in the title outranks a context-only match. Messages use SQLite FTS5 rank; ties resolve by Project name, type, title, and object ID. Project/Task search uses whole Unicode letter/number tokens, not arbitrary substrings. Message snippets are bounded plain text; the CLI prints `N matches; showing first K` and includes the owner, message ref/date/kind/tags, and snippet. Use `--json` for the structured envelope.

`GET .../messages/{messageID}/context` is the pointer-first hydration endpoint. It returns `{anchor, thread, related, revisions}` in one bounded response. Defaults are 12 thread items, 8 related hits, and 4 revisions; hard maxima are 50, 25, and 12. The response includes `*_truncated` flags and `related_mode` (`vector` or `fts`), so an agent can decide whether to request more deliberately rather than silently assuming the bundle is complete. If `q` is omitted, the store derives a small related query from the first tag or a short body prefix and excludes the anchor itself from `related`.

The current `vector` mode is a deterministic feature-hashed cosine index stored as a disposable SQLite accelerator. It is not treated as semantic truth and does not require a model download. The index is rebuilt from `project_messages` when missing or stale; a future LanceDB/neural embedding backend can replace it without changing the durable post/revision contract.

Edits update the current post and append an immutable revision. Thread parentage cannot be edited, and replies must belong to the same Project as their parent. Message creates, replies, and edits also append immutable Project events in the same SQLite transaction.

Pinning is recall metadata, not a content edit: it does not increment `revision`, but `message_pinned` / `message_unpinned` transitions are immutable Project events. Pinned messages sort first in project lists and receive priority in vector-related retrieval.

Agent CLI equivalents:

```text
tk msg P-10000 add --kind observation --tags queue "The queue stalls after Sprint promotion."
tk msg P-10000 add --kind answer --reply-to M-42 "The save boundary throws before processSprint."
tk msg P-10000 edit M-42 --kind lesson "Persist state before promoting a Sprint."
tk msg P-10000 checkpoint --author agent-7 --entry 'decision|Persist before dispatch.|queue' --entry 'lesson|Keep only the root M-ref in active context.|handoff'
tk msg P-10000 pin M-42
tk msg P-10000 unpin M-42
tk msg P-10000 thread M-42
tk msg P-10000 revisions M-42
tk msg P-10000 context M-42 --related-limit 8
tk msg P-10000 search queue Sprint
tk search queue worker --tasks --project P-10000 --limit 10
tk search queue --messages --kind decision --tags durable --after 2026-10-01 --before 2026-10-31
```

Before posting a message with at least four distinct words, `tk msg ... add` performs a read-only, bounded search preflight and writes possible near-duplicate warnings to stderr; the post is never blocked. The preflight is heuristic and may miss a duplicate. If Core search is unavailable, CLI notes that and continues with the requested post.

## Runnable Sprint queue

`POST /api/v1/projects/{projectID}/sprints/claim-next` accepts `{}` and atomically starts the oldest Open Sprint whose Project, Category, Task, and optional Subtask are all Open. It returns the claimed Sprint with `status: "Active"`. It returns `409 no_runnable_sprint` when nothing is eligible. This is deliberate agent pickup, not an implicit scheduler.

Completing a Task-owned Sprint closes its active interval and records the Sprint transition; it does not automatically complete the parent Task. The Task remains `Open` until it is explicitly closed with `POST /api/v1/tasks/{taskID}/status`. Time Keeper rejects manual Task completion while a non-terminal Sprint or Subtask remains.

A Task or Subtask may own one to four Sprints. New Sprints require every owning Project, Category, Task, and optional Subtask to be `Open`; Time Keeper rejects a new child under a terminal or held parent rather than silently reopening parent work. A Task also cannot be manually marked `Completed` while it still owns a non-terminal Sprint or Subtask.

An Open Sprint may transition directly to `On Hold` for any real blocker without creating a work interval. This is the correct state when you run out of road and other items must catch up first, a user decided otherwise, you are just waiting for input, or any external dependency: it remains visible, consumes no active capacity, and does not invent active time.

## Bounded retrieval attempts

`POST /api/v1/sprints/{sprintID}/retrieval-attempts` records immutable evidence that one retrieval cycle did not yield material progress:

```json
{"reason":"Provider returned no usable material after the bounded retrieval."}
```

A Sprint accepts exactly four attempts. The fourth atomically changes it to `TimedOut`, closes any active or held interval honestly, and records both attempt and timeout events. `TimedOut` remains visible and is not runnable; a fifth attempt is rejected. `GET /api/v1/sprints/{sprintID}/retrieval-attempts` returns the complete evidence history.

## Agent usage and token telemetry

`POST /api/v1/projects/{projectID}/usage-sessions/{sessionID}/snapshots` records one cumulative usage snapshot for an agent session. The `sessionID` path segment is the stable external session identity; `agent_id` and `model` remain data fields. A session may optionally be attributed to one Sprint in the same Project.

```http
POST /api/v1/projects/P-10053/usage-sessions/session-42/snapshots
Content-Type: application/json

{
  "agent_id":"codex",
  "model":"gpt-5",
  "sprint_id":"SP-10061",
  "turn_seq":7,
  "input_tokens":12000,
  "output_tokens":1800,
  "cache_creation_tokens":500,
  "cache_read_tokens":2400,
  "context_used":32000,
  "context_size":128000,
  "messages":14
}
```

Counters are cumulative for the session. `turn_seq` must increase. Repeating the same session/turn is an idempotent no-op and returns `duplicate: true` with a zero delta. A provider counter reset requires a new session identity. The response includes the latest cumulative `session` and the newly observed `delta`.

`GET /api/v1/projects/{projectID}/usage-sessions/{sessionID}/snapshots` returns the accepted cumulative snapshots for that Project-owned session in chronological order. An optional non-negative `limit` bounds the returned `items`; an unknown session returns `404 session_not_found`. This readback surface is intended for integration verification and audit tooling, while `usage-summary` remains the dashboard aggregate.

`GET /api/v1/projects/{projectID}/usage-summary` returns the latest cumulative totals for every recorded agent session in the Project. Input, output, cache creation, cache read, context occupancy, and messages remain separate measurements. Cost is a server-owned projection: configure `TIMEKEEPER_MODEL_PRICES_FILE` using the validated format in [`docs/model-pricing.md`](docs/model-pricing.md). Unknown models remain explicitly `cost.status: "unpriced"`; no fallback price is invented.

Usage records are deliberately not Project activity events: they are telemetry projections with their own idempotent ledger. Integrations must redact credentials, prompt bodies, private transcript paths, and tool output before sending snapshots, and must disclose that only sessions which send snapshots are covered. Project export includes the same `usage_summary` projection.

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

When the server is started with an existing private `-guardian-state-dir` (or `TIMEKEEPER_GUARDIAN_STATE_DIR`), it also exposes the canonical recovery callback `POST /api/v1/guardian/recover`. The route accepts the same `timekeeper-pulse-guardian/v1` signal as the legacy receiver: a bounded `application/json` body, exact `recover_attention` action, artifact write, then owner-bound acknowledgement. If recovery is not configured, the route returns `503 guardian_recovery_unavailable` rather than inventing a state directory. `guardian/status` reports the configured callback as `guardian_recovery_url`.

`guardian/status` also returns `callback_migration` so operators can tell whether the compatibility receiver is still needed without parsing each URL. `registered` is the total callback count; `canonical_recovery` counts callbacks matching `guardian_recovery_url`; `legacy_recovery` counts `/v1/recover` artifact callbacks; and `worker_or_custom` counts everything else, including `/pulse` worker-admission callbacks. `legacy_receiver_required` is true only while at least one legacy artifact callback remains, and `canonical_recovery_configured` says whether this Core advertises the canonical route. These counters classify URLs only; they do not prove that a worker process is healthy or that admission succeeded.

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
POST /api/v1/guardian/recover
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

## Memory Galaxy provider boundary

The Obsidian Memory Galaxy integration is optional. TimeKeeper remains fully usable when Obsidian is absent or disconnected. Configure the server with an endpoint descriptor only when the Obsidian plugin is installed:

```text
timekeeper -obsidian-descriptor <path>
# or set TIMEKEEPER_OBSIDIAN_DESCRIPTOR
```

The descriptor is re-read for each provider operation so a plugin restart or dynamically selected port can be observed. It must contain a numeric loopback HTTP `base_url`, the exact `timekeeper-knowledge/v1` protocol, `provider: obsidian`, a vault identity, and a relative or absolute `token_file` reference. The capability token is read from that separate file and is never stored in Galaxy Markdown.

Provider routes are exposed through TimeKeeper:

```text
GET  /api/v1/knowledge/status
POST /api/v1/knowledge/search
GET  /api/v1/knowledge/{id}
```

Search requests and provider responses are bounded. Provider errors use typed codes such as `provider_unavailable`, `invalid_endpoint`, `protocol_mismatch`, and `response_too_large`; endpoint paths, token values, filesystem paths, and raw provider bodies are not returned in errors. With no configured provider, these routes return `503 provider_unavailable` and ordinary TimeKeeper routes continue to work.

The promotion route accepts only an explicit redaction confirmation:

```http
POST /api/v1/projects/P-10000/messages/42/promote
Content-Type: application/json

{"redaction_status":"verified","type":"pattern","visibility":"galaxy"}
```

The Trail entry remains unchanged. TimeKeeper derives an idempotency key from
its stable M-reference and revision, writes a pending `knowledge_links` row,
asks the configured provider to create the item, and updates the mapping to
`created` or a visible retryable/permanent failure. Repeating the request uses
the same key and cannot create a second mapping. Sensitive markers are rejected
before promotion. A promotion without a configured provider returns a typed
`503 provider_unavailable` response.

The provider contract/client, Knowledge read routes, redaction-gated Trail promotion, durable `knowledge_links`, dashboard promotion control, and plugin settings/package contract are implemented and fixture-tested in source. A live Obsidian process smoke and release artifact have not been verified; that work remains On Hold in the Obsidian project. Full Markdown repository semantics remains a separately tracked implementation gap. See [the Memory Galaxy implementation specification](docs/TimeKeeper—Obsidian-Memory-Galaxy-Implementation.md) for the current boundary. Do not describe the integration as fully available until live checks pass.

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

## File history and editor API

File history is optional and is enabled only when the server receives an explicit
`-history-root` (or `TIMEKEEPER_HISTORY_ROOT`) configuration. Without it, file
routes return `503 history_unavailable` and TimeKeeper does not gain implicit
filesystem access.

```text
GET  /api/v1/projects/{projectID}/files
GET  /api/v1/projects/{projectID}/file-root
PUT  /api/v1/projects/{projectID}/file-root
POST /api/v1/projects/{projectID}/files/scan
GET  /api/v1/files/{fileID}
GET  /api/v1/files/{fileID}/versions
GET  /api/v1/files/{fileID}/versions/{versionID}
POST /api/v1/files/{fileID}/versions
POST /api/v1/files/{fileID}/snapshots
GET  /api/v1/files/{fileID}/compare?from=FV-1&to=FV-2
POST /api/v1/files/{fileID}/restore
```

The source root is a separate, per-Project local path; it does not replace the server's
content-addressed history/blob root. Configure it with an absolute path on the TimeKeeper
server's host:

```http
PUT /api/v1/projects/P-10053/file-root
Content-Type: application/json

{"root_path":"D:\\dev\\codebase\\dev\\TimeKeeper"}
```

The server resolves the directory and symlinks before saving the root. The dashboard's
**Scan and index** action calls `POST /api/v1/projects/{projectID}/files/scan` with `{}`.
It discovers bounded UTF-8 text files, skips symlinks, binary/oversize files and common
build/dependency folders, and records new or changed contents as immutable versions.
This is an explicit bounded scan, not a filesystem watcher; edits that occur and disappear
between scans are not guaranteed to be recorded. The scan has limits of 10,000 files and
512 MiB total, with a 10 MiB per-file cap. The service account must have access to the
configured source directory.

A caller can capture a meaningful change boundary without saving every keystroke. Each accepted boundary is stored as an immutable full-content version; the phase is retained at the start of its change summary:

```http
POST /api/v1/files/F-10001/snapshots
Content-Type: application/json

{"phase":"code_start","agent_id":"agent-7","session_id":"S-42","task_id":"T-10331","sprint_id":"SP-10332","change_summary":"Before editor change"}
```

Use `phase: code_end` after the change and verification. Each changed snapshot is a full content-addressed blob linked to its previous version, so every step can be restored independently; these are snapshots, not delta patches. Unchanged `code_change` snapshots do not create duplicate versions. Caller provenance, `change_kind`, and the summary remain on each version record. `change_kind` accepts `milestone`, `bugfix`, `code_change`, `reversion`, `feature`, `release`, `feature_eol`, `file_added`, `refactor`, `test`, `documentation`, `security`, `performance`, `dependency`, `configuration`, or `other`.

Restore requires the target version and the version the editor believes is
current:

```json
{"version_id":"FV-10002","expected_current_version_id":"FV-10003"}
```

A stale expected version returns `409 file_version_conflict`. Restoring does
not erase history; it appends a chronological child of the current version,
sets `change_kind: reversion`, and records the same-file `restores_version_id`
target. The dashboard renders that target as a new side lane while retaining
the old line. The locally bundled CodeMirror 6 adapter (`web/vendor/codemirror-editor.js`) is used through `file-editor.js`; Monaco is not integrated. The project Files view lists tracked file records, previews each recorded version, compares and restores, and exposes a shared icon legend for milestone, bugfix, code change, reversion, feature, release, end-of-life, and other change kinds. It saves a new immutable full snapshot after a short pause in editing, and **Save step now** creates an immediate checkpoint. Edits made outside the dashboard editor are not watched continuously; agent integrations should use the snapshot API. If an outside edit is detected when the editor saves, TimeKeeper records that disk state as a version and returns `409 file_version_conflict` rather than overwriting it. The explorer is limited to tracked files under the explicitly configured history root; it does not enumerate arbitrary Project directories.

## Project export and SQLite backup

`GET /api/v1/projects/{projectID}/export` is a versioned Project-scoped JSON export. It is not a complete database recovery snapshot.

The CLI wraps this API with multi-format file output. `tk export <project-id> <file>` writes the full project hierarchy; the format is detected from the file extension (`.json`, `.csv`, `.tsv`, `.xlsx`, `.yaml`/`.yml`, `.sql`). Without a file argument, JSON is printed to stdout. XLSX produces a multi-sheet workbook (Project, Categories, Tasks, Subtasks, Sprints). CSV produces a ZIP archive with one file per entity type. SQL generates `INSERT INTO` statements.

`POST /api/v1/projects/{projectID}/tasks/batch` and `POST /api/v1/projects/{projectID}/execution-batches` are durable registration offers. Core commits each queue entry before processing it into normal Task/Subtask/Sprint records; the final registration status/result and all registered entities then commit atomically. The legacy validation route and versioned `/execution-batches/validate` route remain read-only smoke gates. Apply returns `201` when registered, `200` for an idempotent replay with the same `Idempotency-Key`, `202` while the durable offer is still queued, and `422` when Core rejects it. Each apply response includes `X-TimeKeeper-Registration-Queue-ID`.

`GET /api/v1/projects/{projectID}/registration-queue?status=Queued&limit=50&offset=0` lists a FIFO page with `has_more` and `next_offset` metadata; `limit` is 1–100 and `offset` is 0–1,000,000. `GET /api/v1/projects/{projectID}/registration-queue/{queueID}` reads the full retained payload, validation, processing state, and registration result. Core retries queued offers after restart. Registered records appear through the ordinary Task/Subtask/Sprint APIs and the Project workspace. Open Sprints then join the separate runnable execution queue consumed by `POST /api/v1/projects/{projectID}/sprints/claim-next`.

The same Core registration queue handles direct `POST` Task, Subtask, Task-Sprint, and Subtask-Sprint routes. Their successful responses retain the normal entity shape and include `X-TimeKeeper-Registration-Queue-ID`; direct-route validation errors preserve the established `400`/`404` statuses while the rejected offer remains in the Core queue with validation details. Callers that send `Idempotency-Key` can safely replay after an uncertain response. The Core worker drains queued offers FIFO while running and after startup recovery. The existing `claim-next` Sprint queue remains a separate downstream execution stage.

The CLI keeps a local client outbox under `.timekeeper/registration-outbox` for direct Task/Subtask/Sprint creates and batch applies. It durably writes each request and stable `Idempotency-Key` before contacting Core; a transport failure or ambiguous response therefore remains retryable instead of vanishing in the ether. `tk queue list` and `tk queue show <local-id>` inspect local receipts; `tk queue flush [--limit N]` submits them FIFO and polls accepted Core queue IDs. This local outbox is distinct from the Core registration queue: only Core-accepted offers appear in the API/Web UI queue. Batch flush always repeats the smoke validation before apply.

`tk t show <task-id> [--json]` and `tk st show <subtask-id> [--json]` use focused detail reads rather than exporting the whole Project. `tk tree <project-id> --task <T-ID>` keeps the owning Category path and selected Task's descendants in the CLI projection; `--status S` filters Task nodes by status while retaining owner Categories; `--active` keeps Active Sprints plus their Task/Subtask owner path; `--depth N` caps the displayed hierarchy (Project is depth 0, Category 1, Task 2, direct Sprint/Subtask 3, and nested Sprint 4; accepted range is 1–10); `--limit N` returns a stable Task prefix and an explicit truncation count. Without `--limit`, `--json` remains the original ExecutionTree shape. Limited JSON wraps the tree with `filters`, `limit`, `total_tasks`, `shown_tasks`, and `truncated`. These are currently client-side filters, so the CLI still downloads the complete execution-tree response before filtering. `tk tree <project-id> --help` and `tk summary <project-id> --help` are local, non-network help requests.

`tk t export <project-id> <file>` fetches tasks via `GET /api/v1/projects/{projectID}/tasks` and writes them to any supported format. Optional `--status` and `--category` flags filter the output client-side.

Use server backup mode for a recoverable SQLite snapshot:

```text
timekeeper -db timekeeper.db -backup-to timekeeper-backup.db
```

The destination must not exist. `Store.BackupTo` first runs SQLite `PRAGMA integrity_check` on the primary; if it is not exactly `ok`, it refuses to create a backup. The one-shot server mode verifies the resulting candidate read-only before reporting success. Backups use `VACUUM INTO`, request owner-only mode on Unix-like filesystems, and remain unencrypted.

The long-running server also maintains two A/B local shadow snapshots by default in `recovery/sqlite-shadow/` beside the primary, every five minutes. Each rotation checks the primary before snapshotting, verifies the candidate and SHA-256 before replacing the inactive slot, and records the active slot in a manifest. Set `-shadow-copy-dir` to place copies on a separate volume, `-shadow-copy-interval 0` to disable, or choose a cadence of at least one minute.

To restore, stop Core and run the server binary in offline maintenance mode with an explicit slot and confirmation: `-restore-shadow-slot shadow-a.db -confirm-shadow-restore`. The command refuses if the database lock or configured loopback listener is busy (default `timekeeper.local:1618`); it verifies the candidate, preserves the current `.db`, `-wal`, and `-shm` set, installs the candidate, checks the installed primary, and exits without restarting Core. See `docs/shadow-copy.md` for the full safety contract.
