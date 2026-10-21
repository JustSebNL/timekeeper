# Queue logic: Tasks, Sprints, and the runnable queue

## Authority
This document describes the durable model enforced by TimeKeeper (SQLite store + API). The code, not this text, is the source of truth; if they diverge, trust `internal/store/store.go`, `internal/model/`, and `docs/API.md`.

## Registration queue (incoming offers)
Task, Subtask, Sprint, and batch-creation API requests are first appended as durable registration offers. Each offer is FIFO, has an idempotency key when supplied, and remains visible with `Queued`, `Registered`, or `Rejected` state. Core processes each offer through the existing API validation and registration semantics; all entities in one batch and that queue item's final result/status commit atomically. A crash before processing leaves the offer queued; a crash during registration rolls back the entities and leaves the offer retryable. Rejected offers remain inspectable with structured validation findings rather than vanishing.

This is the **registration queue**: it moves an API offer into the durable hierarchy and Web UI. The separate **execution queue** below is the FIFO set of runnable Open Sprints claimed by agents. Once an offer registers, its Open Sprints become execution-queue work; Tasks without Sprints remain durable plans, not runnable items. Do not create a second in-memory Task queue or silently treat a queued registration as already registered.

The Core list API is FIFO-paged (`limit`/`offset` with `has_more`/`next_offset`); the Web UI must expose older pages rather than silently stopping at its first page. Queue history remains queryable by status and item ID.

The CLI adds a pre-Core **local outbox** for Task/Subtask/Sprint and batch-create commands. It writes the immutable request payload and stable idempotency key under `.timekeeper/registration-outbox` before the network call, with append-only state snapshots. `tk queue list`/`show` inspect this local outbox; `tk queue flush` retries FIFO using the same key and re-runs batch smoke before apply. Local outbox entries are not yet Core records and cannot reach the Web UI until Core accepts them. Keep this boundary explicit: local outbox → Core registration queue → durable Project records → Sprint execution queue.

## Hierarchy
Project (`P-`) → Category (`C-`) → Task (`T-`) → Subtask (`ST-`, optional) → Sprint (`SP-`).
Every Sprint belongs to exactly one Project. It belongs directly to a Task, or to a Subtask under that Task. There is no Sprint that floats independently of a Project/Category/Task (or Subtask).

## Tasks are Sprint containers
A Task or Subtask may own **one to four Sprints**. This is the bounded container rule. It means:

- A Task with zero Sprints is an open plan with no runnable work queued.
- A Task with four Sprints is at full capacity; no additional Sprint may be added to that Task (the store rejects the creation).
- Completing or cancelling a Sprint does not automatically complete its parent Task. Task status is explicit; close the Task with `tk t status <task-id> Completed` only after its scope is actually finished.
- A Task with any non-terminal Sprint or Subtask cannot be manually marked `Completed`; after its runnable work is exhausted it remains `Open` until an explicit status change.

## Sprint lifecycle
A Sprint moves through discrete states. The allowed transitions are enforced by the store; the API returns `409` for illegal moves rather than silently ignoring them.

States: `Open` → `Active` (via `start` or `claim-next`) → `On Hold` (via `hold` with a reason) → `Active` (via `resume`) → `Completed` (via `complete` with evidence) or `TimedOut` (after 4 failed retrieval attempts) or `Cancelled` (explicit cancel with a reason).

Rules that matter:
- A Sprint cannot transition directly from `Open` to `Completed` without an intervening `Active` interval (the lifecycle is `start → work → complete`). Calling `complete` on an `Open` Sprint without a `start` fails; always: `sp start SP-N "<reason>"`, do the work, then `sp complete SP-N "<evidence>"`.
- `On Hold` does not invent active work time; it is the correct state when you hit a real blocker (other items must catch up, a user decided otherwise, you are waiting for input, or any external dependency). It consumes no active capacity and does not trigger overdue nudges.
- `TimedOut` is terminal. After exactly 4 retrieval attempts, the 4th atomically changes the Sprint to `TimedOut`, records the attempt, and closes any active/held interval honestly. `TimedOut` remains visible but is not runnable; a 5th attempt is rejected.
- `Open` Sprints whose parent Project, Category, Task, or Subtask is not `Open` are non-runnable. They stay visible but cannot be started; the `attention` endpoint (read-only decision queue) surfaces them alongside `On Hold` and `TimedOut` items.

## Queue behavior (what stays, what leaves, what times out)
The runnable queue is the set of Sprints that are eligible for `claim-next`: all `Open` Sprints whose entire parent chain (Project, Category, Task, optional Subtask) is `Open`, sorted by creation order, with the oldest eligible Sprint claimed first. This is deliberate agent pickup (`POST /api/v1/projects/{projectID}/sprints/claim-next`), not an implicit scheduler.

When a Sprint completes:
- The Sprint's interval is closed and the completed Sprint remains in durable history; it leaves the runnable queue.
- The parent Task remains `Open`, even when this was its last Sprint. Once the Task's scope is verified complete, close it explicitly with `tk t status <task-id> Completed`.

When a Sprint is held (`On Hold`):
- It leaves the runnable queue until it is resumed. It does not time out; timeout only applies to `Active` work exceeding planned duration plus approved extensions.
- It remains visible in the `attention` read-only view (with its recorded reason), separate from the `pulse` overdue surface.

When a Sprint times out (`TimedOut`):
- It is removed from the runnable queue and from overdue attention. It remains visible in durable history. It cannot be restarted; only a new Sprint (under an open parent) can replace the work.

When a Sprint is cancelled:
- It is terminal (like `TimedOut`). It requires an explicit cancel reason. It is not deleted; it stays in durable history and is excluded from the runnable queue.

## Bounded container enforcement (1–4 Sprints per Task/Subtask)
The store enforces this at creation time. A new Sprint requires:
- Parent Project `Open`
- Parent Category `Open`
- Parent Task `Open`
- Parent Subtask `Open` (if owned by a Subtask)
If any parent is terminal (`Completed`, `TimedOut`, `On Hold` for the parent, or `Cancelled`), the Sprint creation is rejected rather than silently reopening parent work.

A Task also cannot be manually marked `Completed` while it still owns a non-terminal Sprint or Subtask; the store checks this before applying the status change.

## Timeout vs. queue removal (distinction)
- Timeout (`TimedOut`): a Sprint that had 4 failed retrieval attempts. It leaves both the runnable queue and the overdue attention surface.
- Queue removal (still active but not runnable): a Sprint owned by a non-`Open` parent (e.g., a Task that has been completed or held). It is not started; the `attention` endpoint surfaces it for manual review.
- Queue removal (held): a Sprint moved to `On Hold`. It is not started; it remains in `attention` but does not trigger overdue nudges; it does not count as queued work until it is resumed.

## Verification
Verify queue behavior by inspecting the live state, not by reading this file:

```bash
# Current runnable and overdue state
./.timekeeper/app/bin/tk pulse

# Full execution tree for a project (shows Sprint ownership and parent status)
./.timekeeper/app/bin/tk tree P-10053

# Read-only attention (held, timed out, and non-runnable open items)
# This is an API surface; the CLI reflects it through pulse and summary output.
curl -sS http://timekeeper.local/api/v1/projects/P-10053/attention | head
```

A Sprint that claims to be completed but is still listed in the runnable queue, or a completed parent that still has open queued Sprints, indicates a divergence between this description and the store state. When that happens, the store state (verified through `tk tree`, `tk pulse`, and the `attention` endpoint) is the truth; this document must then be corrected.
