# Jev KB — Decision Policy, Abstention, and Safety

## Role separation

```text
Jev: typed judgement over bounded candidates.
Policy code: thresholds, risk, contradiction detection, and action composition.
TimeKeeper: authorization, validation, persistence, lifecycle, and readback.
ShadowBox: isolated TypeSafe call, credential, egress, timeout, and artifact.
```

Jev is not an agent, database, scheduler, shell, or approval bypass.

## Two-pass policy

Use this sequence:

```text
1. Resolve Project exactly.
2. Fetch and flatten authoritative execution tree.
3. Jev chooses Category or review.
4. Policy validates Category, status, and Project ownership.
5. Narrow Tasks/Subtasks/Sprints to that Category.
6. Jev chooses existing target, new Task/Sprint, or review.
7. Policy validates target, capacity, lifecycle, duration, risk, and confirmation.
8. TimeKeeper performs the approved mutation.
9. Read back exact state and record outcome.
```

Do not let a second question select a child whose candidates depend on a sibling answer in the same parallel TypeSafe request.

## Typed question types

### Choice

Use for one selection from bounded candidates:

```text
category_target
project_target
task_target
sprint_target
work_disposition
```

Always include an escape option where forcing a candidate is unsafe.

### Score

Use for an ordered bounded dimension:

```text
scope_fit
risk_level
sprint_size
evidence_quality
```

Define every level concretely. Do not use an unbounded number for Sprint duration.

### Noul

Use for one proposition:

```text
requires_new_category
requires_subtask
can_start_now
needs_code_snapshots
```

Noul returns a probability of the proposition; do not expect a separate confidence field.

## Confidence gates

Initial gates are calibration placeholders:

```text
Read-only Category suggestion: >= 0.70
Existing Category ordinary routing: >= 0.85
Resume existing Sprint: >= 0.85 for disposition and target
Create Sprint on existing Task: >= 0.90 plus capacity and confirmation in Stage 1
Create Task and Sprint: >= 0.90 plus confirmation
Create Category: >= 0.90, no existing candidate above 0.50, confirmation always
High-risk/history-changing actions: never Jev-only
```

Inspect the complete probability distribution and top-two margin. A high confidence result cannot override a status, parentage, capacity, or permission failure.

## Abstention rules

Return `needs_review` when:

- Project is missing, ambiguous, or closed;
- Category candidates tie or none has durable scope fit;
- the candidate list was truncated without a safe deterministic narrowing result;
- selected action has no target;
- Category and Task/Sprint parentage conflict;
- selected entity is foreign to the Project;
- selected Sprint is not runnable for the requested action;
- Task/Subtask has four direct Sprints;
- parent status is not Open for creation;
- answer is missing, blank, malformed, or unknown;
- answers contradict one another;
- state changed after decision and has not been refreshed;
- human confirmation is required but not present;
- ShadowBox policy or TypeSafe availability is unverified.

`needs_review` is the safe result. Never fall through to the first candidate.

## Contradiction examples

```text
work_disposition = resume_existing
sprint_target = none_of_the_above
```

```text
category_target = C-10001
requires_new_category = true
```

```text
create_sprint_on_existing_task
selected_task = T-... from another Category
```

```text
new Sprint requested
owner direct_sprint_count = 4
```

Reject all of these as executable proposals.

## Mutation policy

Jev cannot mutate. The adapter must build a typed proposal containing:

```text
decision_id
protocol_version
project_id
category_id
task_id/subtask_id
sprint_id or proposed Sprint fields
action
confidence evidence
policy version
agent/session provenance
idempotency key
```

Before mutation:

- reread the Project tree;
- verify every ID and parent;
- verify status and capacity;
- verify the request is still authorized;
- use the actual TimeKeeper route;
- use idempotency.

After mutation:

- read back the exact entity;
- verify parents, status, name, estimate, and event;
- record `applied` only after readback.

## ShadowBox boundary

The Jev call runs behind ShadowBox. ShadowBox owns the TypeSafe credential under:

```text
TYPESAFE_API_KEY
```

Required controls:

- allowlist the TypeSafe HTTPS destination;
- deny arbitrary destinations;
- bound request/response size;
- bound timeout and transient retries;
- redact credential and state from logs/errors;
- do not expose the key to TimeKeeper Core;
- do not let Jev write SQLite or execute shell commands;
- fail closed if the connector cannot enforce policy;
- store only a sanitized run/decision artifact.

TimeKeeper remains usable through manual CLI/API operation if ShadowBox or Jev is unavailable.

## Privacy and logging

Normal logs may contain:

```text
decision_id
stage
status
latency
outcome
bounded error code
```

Debug logs may contain bounded redacted metadata. Never log:

```text
TYPESAFE_API_KEY
bearer tokens
full prompts or transcripts
unbounded source/log bodies
private connection strings
```

Project messages have a bounded body. Store large decision state as a local/private artifact or dedicated record and put only a compact M-reference in the Project board.
