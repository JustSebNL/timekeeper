# Jev KB — Detecting, Resuming, and Creating Sprints

## Objective

Decide whether the intent should resume an existing Sprint, create a new Sprint under an existing Task/Subtask, or stop for review.

A Sprint is a bounded execution cut. It should produce one independently verifiable outcome. If the work is larger than one bounded cut, split it into multiple Sprints instead of inventing a giant estimate.

## Detecting an existing Sprint

Prefer an existing Sprint when:

1. Its Project, Category, Task/Subtask, and scope match the intent.
2. It is Open and runnable, Active and already owned by the current actor, or On Hold with a valid resume reason and policy permission.
3. The requested work is a continuation, verification, correction, or completion of that Sprint's goal.
4. Resuming it will not mix unrelated deliverables.
5. The Sprint has not become terminal, stale, or invalid through parent status changes.

Candidate fields:

```text
sprint_id
project_id
category_id
task_id
subtask_id
item_address
name
description
goal
status
priority
estimated_duration_seconds
buffer_pct
active_duration_seconds
hold_duration_seconds
hold_reason
started_at / ended_at
runnable
recent evidence
```

A matching name is not enough. Compare the Sprint goal and remaining scope.

## Runnable Sprint definition

An Open Sprint is runnable only when its complete parent chain is Open:

```text
Project = Open
Category = Open
Task = Open
Subtask = Open, if present
Sprint = Open
```

The oldest eligible Open Sprint is returned by the deliberate `claim-next` operation. This is agent pickup, not an implicit background scheduler.

An Active Sprint is already being worked and must not be claimed by a second actor. An On Hold Sprint is visible for attention but is not runnable until resumed. Completed, TimedOut, and Cancelled Sprints are terminal history.

## When to create a new Sprint

Create a new Sprint only when:

```text
The Task/Subtask is the correct owner.
No existing Sprint safely matches the intent.
The complete parent chain is Open.
The owner has fewer than four direct Sprints.
The proposed outcome is independently verifiable.
The estimate is positive and bounded by policy.
The name and goal are specific.
The mutation is idempotent and approved.
```

A new Sprint is created under a Task or Subtask, never directly under a Category or Project.

## Hard Store rules

The current Store enforces:

- Sprint name is required after trimming;
- `estimated_duration_seconds` must be positive;
- `buffer_pct` must be a whole number from 0 through 100;
- all owning parent statuses must be Open;
- a Task or Subtask may own at most four direct Sprints;
- new Sprints start with status Open;
- completion requires an earlier Active interval.

These validations are authority. Jev may recommend a value, but policy must validate it before sending a request.

## Sprint sizing

Use the smallest estimate that can produce one verifiable outcome. Recommended policy bands are configurable and must not override Store validation:

```text
small: focused inspection or one small change
medium: bounded implementation plus focused tests
large-but-bounded: implementation plus integration verification
oversized: split into multiple Sprints or review
```

Do not translate a broad project goal into one Sprint. Split by independently verifiable outcomes, for example:

```text
Sprint 1: reproduce and capture the failure
Sprint 2: implement the narrow fix
Sprint 3: add regression coverage
Sprint 4: verify installed/runtime behavior
```

The one-to-four limit is per direct owner. When the fourth Sprint is already present, select existing work, use a Subtask with capacity if valid, or propose a follow-on Task. Never create a fifth Sprint.

## Creation routes

Use the actual API routes:

```text
POST /api/v1/tasks/{taskID}/sprints
POST /api/v1/subtasks/{subtaskID}/sprints
```

Example body:

```json
{
  "name": "Implement queue container fix",
  "description": "Keep Tasks durable while their Sprints execute.",
  "goal": "Verified Task queue behavior with regression coverage.",
  "priority": "Normal",
  "estimated_duration_seconds": 1800,
  "buffer_pct": 10
}
```

Do not guess a Category-Sprint route. The selected Task/Subtask is the parent.

## Lifecycle

```text
Create: Open
Start or claim-next: Active
Hold with reason: On Hold
Resume: Active
Complete with evidence: Completed
Fourth failed retrieval attempt: TimedOut
Cancel with reason: Cancelled
```

Required execution pattern:

```text
start -> bounded work -> verify -> record evidence -> complete
```

An Open Sprint cannot jump directly to Completed. An On Hold Sprint does not consume active time and does not trigger overdue work in the same way as Active work.

## Resume versus create decision

Use `resume_existing` when an existing Sprint's remaining scope matches. Use `create_sprint_on_existing_task` when the Task is correct but no Sprint matches and capacity exists. Use `create_task_and_sprint` only when no existing Task owns the durable outcome.

If the answer says `resume_existing` but no concrete Sprint ID is selected, the proposal is invalid and becomes `needs_review`.

## Output

```json
{
  "action": "resume_existing | create_sprint | create_task_and_sprint | needs_review",
  "task_id": "T-10005",
  "subtask_id": null,
  "sprint_id": "SP-10006",
  "estimated_duration_seconds": null,
  "buffer_pct": null,
  "runnable": true,
  "confidence": 0.90,
  "reason": "Existing Open Sprint goal matches the requested bounded continuation.",
  "requires_confirmation": false
}
```

For a new Sprint, `sprint_id` is absent until TimeKeeper creates it and readback verifies it.

## Common mistakes

- Creating a Sprint directly below a Category.
- Creating a fifth direct Sprint.
- Claiming an Active Sprint owned by another actor.
- Resuming a terminal Sprint.
- Treating an On Hold Sprint as runnable without explicit resume policy.
- Completing without starting.
- Choosing a duration because the model likes a round number rather than because the outcome is bounded.
- Treating a successful create response as proof without readback.
