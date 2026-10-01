# Jev KB — Decision Output Contract

## Purpose

This is the contract between Jev's typed answer, deterministic policy, and TimeKeeper's validated mutation path.

Jev returns typed answers. The adapter converts them into a proposal. TimeKeeper decides whether the proposal is structurally and operationally valid.

## Decision envelope

```json
{
  "version": "timekeeper-jev-decision/v1",
  "decision_id": "DEC-...",
  "mode": "propose | confirm | review",
  "outcome": "proposed | needs_review | confirmed | applied | rejected | failed",
  "project_id": "P-10000",
  "category_id": "C-10001",
  "task_id": "T-10005",
  "subtask_id": null,
  "sprint_id": "SP-10006",
  "action": "resume_existing",
  "confidence": {
    "project": 0.98,
    "category": 0.91,
    "task": 0.88,
    "sprint": 0.90
  },
  "probabilities": {
    "category_target": {},
    "task_target": {},
    "sprint_target": {},
    "work_disposition": {}
  },
  "reason": "bounded candidate-grounded explanation",
  "requires_confirmation": false,
  "policy_version": "jev-routing-policy/v1",
  "model_version": "jev-latest",
  "candidate_state_hash": "sha256:...",
  "intent_hash": "sha256:...",
  "idempotency_key": "jev:session:intent-hash",
  "provenance": {
    "agent_id": "agent:worker-7",
    "session_id": "session-42",
    "shadowbox_run_id": "run-..."
  }
}
```

Fields must be omitted or null when they are not valid for the action. Do not emit a fake Sprint ID for a proposed creation.

## Action values

```text
select_project
existing_category
create_new_category
existing_task
existing_subtask
resume_existing
create_sprint
create_task_and_sprint
hold_for_review
needs_review
```

The implementation may use a narrower enum, but it must preserve the distinction between:

```text
read-only recommendation
proposal requiring confirmation
approved mutation
applied mutation verified by readback
```

## Review result

```json
{
  "version": "timekeeper-jev-decision/v1",
  "outcome": "needs_review",
  "action": "needs_review",
  "project_id": "P-10000",
  "candidate_ids": ["C-1", "C-2"],
  "reason_code": "ambiguous_category",
  "reason": "Two Categories have similar scope and the intent does not identify the product.",
  "questions_for_human": ["Which product owns this change?"],
  "retryable": false
}
```

Valid reason codes include:

```text
project_not_found
ambiguous_project
ambiguous_category
no_category_fit
ambiguous_task
no_task_fit
no_sprint_target
parent_not_open
sprint_capacity_exhausted
sprint_not_runnable
stale_candidate
conflicting_answers
missing_answer
shadowbox_unavailable
jev_unavailable
confirmation_required
```

## Proposal-to-mutation rules

A proposal may be applied only when:

```text
outcome = proposed or confirmed
all required IDs are present
all IDs belong to the same Project and expected parents
candidate state is current or refreshed
status and capacity checks pass
confidence/risk policy passes
confirmation exists where required
idempotency key is new or matches the same intent
```

TimeKeeper must reject an invalid proposal. The adapter must surface the structured error instead of claiming success.

## Readback contract

After a mutation, read back the exact entity and verify:

```text
returned ID
Project ID
Category ID
Task/Subtask parent ID
Sprint status
name/goal/estimate
created or transition event
idempotency result
```

Only then set:

```text
outcome = applied
```

## Audit record

Record at minimum:

```text
decision_id
protocol_version
project_id
agent_id
session_id
shadowbox_run_id
intent_hash
candidate_state_hash
selected IDs
bounded answer distributions
model/policy versions
thresholds
outcome
idempotency_key
error/review code
created/applied timestamps
```

Never store the TypeSafe credential, bearer token, or unrestricted raw state in the record.

## Human-facing explanation

A concise explanation must answer:

```text
Where did the work go?
Why that Project/Category/Task/Sprint?
Was existing work resumed or new work proposed?
What confidence/risk gate applied?
Was anything mutated?
What evidence or confirmation is required next?
```

The explanation must remain grounded in the candidate facts. It must not claim that Jev inspected files or performed work unless the runtime actually supplied that evidence.
