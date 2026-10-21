# Jev KB — Queue and Lifecycle Logic

## Core queue model

TimeKeeper has two related but different queue concepts:

```text
Task queue: durable containers that remain in the Task list.
Sprint queue: bounded execution items that become runnable/Active.
```

A Task does not leave the Task queue when one Sprint starts or completes. A Task is a container for one to four direct Sprints, plus optional Subtasks that own their own Sprints.

The runnable queue contains eligible Sprints, not disappearing Tasks.

## Hierarchy

```text
Project -> Category -> Task -> optional Subtask -> Sprint
```

Every runnable Sprint must have an Open parent chain.

## Task container rules

- A Task may own one to four direct Sprints.
- A Task with zero Sprints is an open plan with no queued executable cut.
- A Task with four direct Sprints is at capacity for direct Sprint creation.
- Subtask-owned Sprints do not count as direct Task-owned Sprints.
- Completing a Sprint does not automatically complete the Task.
- A Task remains Open until its scope is verified and it is explicitly completed.
- A Task with non-terminal Sprints/Subtasks cannot be manually completed.
- A completed Task is a delivery boundary. New unrelated work gets a follow-on Task rather than reopening history.

## Runnable Sprint queue

The runnable set is:

```text
all Open Sprints
whose Project is Open
whose Category is Open
whose Task is Open
whose optional Subtask is Open
```

`POST /api/v1/projects/{projectID}/sprints/claim-next` atomically starts the oldest eligible Open Sprint and returns it Active. If none is eligible, the API returns a structured no-runnable result/conflict. Claiming is deliberate agent pickup, not an invisible scheduler.

## Sprint transitions

```text
Open -> Active
Open -> On Hold (real blocker and reason)
Active -> On Hold (real blocker and reason)
On Hold -> Active (resume)
Active -> Completed (verified evidence)
Active -> TimedOut (fourth failed retrieval attempt)
Open/Active/On Hold -> Cancelled (explicit reason where allowed)
```

The exact allowed transitions are enforced by the Store. Jev must not recommend an unlisted transition.

## Queue meaning of each state

### Open

Queued but not actively executing. It is runnable only if all parents are Open. It can be claimed or explicitly started.

### Active

Currently executing or claimed. It has an active interval and should have progress/evidence. Another actor must not claim it.

### On Hold

Not runnable. Use when blocked, waiting for input, waiting for another item, superseded temporarily, or otherwise unable to proceed. Record a useful hold reason. Hold does not invent active work time.

### Completed

Terminal successful history with evidence. It leaves the runnable queue but remains visible.

### TimedOut

Terminal failed retrieval history. The fourth retrieval attempt changes the Sprint to TimedOut. It cannot be restarted; create a new Sprint under an eligible open parent after scope review.

### Cancelled

Terminal intentionally stopped history. It is not deleted or runnable.

## Queue removal is not always completion

Distinguish:

```text
Completed: work finished and evidence recorded.
TimedOut: retrieval attempts exhausted.
Cancelled: explicit cancellation.
On Hold: temporarily not runnable, still awaiting a decision.
Non-runnable Open: visible work whose parent is not Open.
```

A Sprint can leave the runnable queue without being completed. Jev must use the state and reason, not only absence from a queue response.

## Attention surfaces

Use:

```text
GET /api/v1/pulse
```

for Active Sprint deadline/overdue attention.

Use:

```text
GET /api/v1/projects/{projectID}/attention
```

for held, timed-out, and non-runnable open work that needs a decision. These are read-only surfaces; they do not start or resume work.

Use:

```text
GET /api/v1/projects/{projectID}/operational-summary
```

for aggregate Open/Active/Held/Completed counts when deciding whether a project still has runnable work.

## Jev queue decisions

Jev may recommend:

```text
claim/resume existing Sprint
create a new Sprint under an eligible owner
hold for blocker/review
create a follow-on Task
needs_review
```

Jev must not:

```text
silently reopen a parent
delete a terminal Sprint
skip the Active interval
create work under a closed parent
use queue absence as proof of completion
```

## Verification commands

```bash
./.timekeeper/app/bin/tk pulse
./.timekeeper/app/bin/tk tree P-10053
curl -sS http://timekeeper.local/api/v1/projects/P-10053/attention
curl -sS http://timekeeper.local/api/v1/projects/P-10053/operational-summary
```

If docs and runtime state disagree, trust the Store/API, capture the discrepancy, and update the knowledge base after the source behavior is understood.
