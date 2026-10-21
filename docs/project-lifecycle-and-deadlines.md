# Project Lifecycle and Deadline Contract

This document defines TimeKeeper's Project-level journey from intake to closeout. Projects remain the durable scope container; Categories/Tasks/Subtasks/Sprints remain the execution hierarchy. Project phases, deadlines, and milestones add schedule and lifecycle visibility without replacing execution records.

## Lifecycle

A Project moves through these explicit phases:

1. **Intake** — capture the problem, intended outcome, and initial owner/context.
2. **Planning** — set the target window and dated milestones; decompose work into Categories, Tasks, and bounded Sprints.
3. **Execution** — deliver work through existing Sprint start/work/complete transitions.
4. **Validation** — verify deliverables against acceptance criteria and record evidence.
5. **Closeout** — summarize outcome, record actual completion/cancellation, and preserve the Project's history.

Phase is distinct from current Project status. Preserve existing statuses (`Open`, `On Hold`, `Completed`, `Cancelled`) and their callers; changing phase must not silently reopen or complete a Project. Actual start/close timestamps are set by explicit lifecycle actions and are immutable once recorded except through a separately audited correction.

## Schedule fields and date rules

Projects should expose:

- `planned_start_date` — optional calendar date (`YYYY-MM-DD`), without a timezone conversion.
- `target_end_date` — optional calendar due date (`YYYY-MM-DD`).
- `actual_started_at` — nullable UTC timestamp set on the explicit transition into execution.
- `completed_at` — nullable UTC timestamp set when the Project is completed.
- `cancelled_at` — nullable UTC timestamp set when the Project is cancelled.
- `lifecycle_phase` — one of the lifecycle phases above.

Calendar dates are not durations. Estimates remain workload quantities, and Sprint lease deadlines remain operational timers; neither may be relabeled as a Project due date. Reject malformed dates and planned start dates after the target end date. Clearing an optional planned date is explicit and audited.

Deadline state is derived rather than stored: `unscheduled`, `on_track`, `due_soon`, `overdue`, `completed`, or `cancelled`. A due-soon warning window is configurable per Project, defaults to seven calendar days, and compares date-only values in the Project's selected IANA timezone (default `UTC`). Completed/cancelled Projects are not overdue. Do not invent a projected finish date from remaining duration unless capacity and calendar assumptions are explicit.

## Milestones

A Project may have dated deliverable checkpoints independent of its Tasks:

- stable milestone ID, name, optional description/acceptance text;
- target calendar date and optional actual completion timestamp;
- status: `Planned`, `In Progress`, `Completed`, or `Cancelled`;
- optional link to an existing Task/Subtask as its execution source.

Milestones express *when an outcome is expected*; linked Tasks/Sprints express *how it is executed*. A milestone is never a second copy of a Task. Completion records evidence and actual completion time; moving a target date preserves the previous value in the audit trail.

## API and agent CLI

The durable Project schedule is exposed through the existing Project JSON objects (list, execution tree, and export). Mutations are explicit and project-scoped:

- `GET /api/v1/project-deadlines` returns an overdue-first dashboard watch of open Project and milestone deadlines, including entity IDs, dates, state, and days remaining.
- `PATCH /api/v1/projects/{projectID}/schedule` updates planned start, target end, schedule timezone, and warning window; omitted fields are preserved and an empty date clears it.
- `POST /api/v1/projects/{projectID}/phase` transitions one lifecycle phase; backward movement requires a reason.
- `GET|POST /api/v1/projects/{projectID}/milestones` lists or creates milestones.
- `PATCH /api/v1/projects/{projectID}/milestones/{milestoneID}` edits details/date/task link.
- `POST /api/v1/projects/{projectID}/milestones/{milestoneID}/status` applies a validated, audited milestone transition.
- Project export includes the schedule, milestone set, and activity events.

Agent CLI entrypoints mirror these contracts: `tk p schedule`, `tk p phase`, `tk p deadlines`, and `tk p milestones list|add|date|status`. CLI output should include stable IDs, explicit due dates, derived schedule state, and status; it must not substitute estimates for calendar dates.

## Timeline and deadline visibility

The Project view should show a chronological lifecycle timeline containing phase/status transitions, planned start/due dates, milestones, Task/Sprint completion evidence, and final closeout. The overview should separately summarize overdue and due-soon milestones/projects with explicit dates and actionable links. Status colors/icons are presentation only; the API-derived dates and state remain authoritative.

The deadline watch is a dashboard alert surface, not a push notification. Any future notification delivery must be opt-in/configurable, deduplicated per Project/milestone/date window, and delivered only through existing configured notification channels. A dashboard alert must not fabricate a durable notification record. Never auto-complete work merely because a deadline passes.

## Closeout

Completion should require an explicit user/agent action and evidence summary. The closeout view should show completed, open, and cancelled milestones; Tasks/Sprints remaining; the actual end date; and links to the durable Project activity/Trail of Thought. Cancellation requires a reason. Closeout never deletes execution records or history.

## Implementation slices

The mapped TimeKeeper Task is `T-11510` in `P-10053`, Category `C-11509`:

- **SP-11512** — audit existing lifecycle and settle dates, milestone, deadline, and closeout semantics.
- **SP-11513** — additive schema, validation, Store/API/CLI contracts for Project dates, lifecycle transitions, and milestones.
- **SP-11514** — dashboard Project timeline, due-soon/overdue visibility, and lifecycle controls.
- **SP-11515** — notification/dedup behavior, migrations, docs, tests, and live verification.

Keep every slice independently testable. A source-level route test proves the source contract only; after rollout, read the new fields and route responses back through `http://timekeeper.local` before calling the feature live.

## Acceptance criteria

- Existing Project status semantics and old database migrations remain compatible.
- Planned calendar dates round-trip without timezone drift; actual timestamps are UTC.
- Invalid date order, invalid phase/status transitions, and invalid milestone links return structured validation errors.
- Milestone changes and Project lifecycle changes create durable, project-scoped audit events.
- Deadline state is derived from stored dates and explicit configuration; no estimate is silently converted to a date.
- Dashboard and CLI/API expose the same persisted schedule and milestones.
- Deadline alerts, if enabled, are idempotent and do not auto-mutate Project/Task/Sprint status.
- Completion/closeout is explicit, preserves history, and displays unresolved work honestly.
- Run focused migration/Store/API/UI tests, then `go vet ./...`, `go test ./...`, `go build ./...`, and verify friendly-origin live routes after rollout.
