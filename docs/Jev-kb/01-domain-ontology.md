# Jev KB — TimeKeeper Domain Ontology

## What each entity means

### Project (`P-`)

A Project is the top-level unit of tracked work. It has a durable ID, optional exact alias, name, goal, description, priority, status, and calculated/reported completion.

Use a Project when work has its own durable objective, ownership boundary, history, and execution tree. Do not choose a Project merely because its name contains a keyword from the intent.

Project statuses are explicit:

```text
Open | On Hold | Completed | Cancelled
```

A Project is the scope boundary for all child entities and decision candidates.

### Category (`C-`)

A Category is an organizational work area inside one Project. It has:

```text
category_id
project_id
parent_category_id (optional)
name
description
goal
status
priority
progress
```

Categories may be nested. A nested Category still belongs to the same Project. Its parent is organizational context, not a different Project boundary.

### Task (`T-`)

A Task is a substantial outcome inside one Category. It has:

```text
task_id
project_id
category_id
name
description
goal
status
priority
estimated_duration_seconds
completion projections
```

A Task is a container for direct Sprints. It remains a durable Task record while its Sprints move through the runnable queue. Do not treat a Task as the queue item that disappears after one Sprint.

### Subtask (`ST-`)

A Subtask is an optional smaller owner below a Task. It inherits Project and Category scope through its parent Task and may own its own direct Sprints. If automatic Subtask creation is not explicitly enabled, route a need for a new Subtask to review rather than inventing one.

### Sprint (`SP-`)

A Sprint is a bounded timed execution cut under a Task or Subtask. It has:

```text
sprint_id
project_id
category_id
task_id
subtask_id (optional)
name
description
goal
status
priority
estimated_duration_seconds
buffer_pct
buffer_duration_seconds
active_duration_seconds
hold_duration_seconds
hold_reason
started_at / ended_at
```

A Sprint is the item that becomes runnable/Active. It is not a free-floating work item and it cannot be created directly under a Category.

## Hierarchy invariant

```text
Project
  -> Category
      -> Task
          -> optional Subtask
              -> Sprint
```

Direct Task Sprint ownership and Subtask Sprint ownership must remain distinct in candidate state. Do not merge them into one unlabelled list.

## Identity rules

- IDs are authoritative; names are display text.
- Never manufacture `P-`, `C-`, `T-`, `ST-`, or `SP-` values.
- Resolve an alias to exactly one canonical ID before asking Jev to route work.
- Every selected child must be verified against the selected Project.
- `item_address` is useful for human explanation but does not replace the stable ID.
- A model-selected name is not proof that an entity exists.

## Status vocabulary

Projects, Categories, and Tasks use explicit workflow states such as:

```text
Open | On Hold | Completed | Cancelled
```

Sprints have a distinct lifecycle:

```text
Open -> Active -> On Hold -> Active -> Completed
                    |                 |
                    +-> TimedOut      +-> Cancelled
```

The Store/API may reject illegal transitions with a conflict. Jev must recommend only transitions supported by the current route contract.

## Parentage facts Jev must retain

When a candidate is sent to Jev, include enough parent data to reject false matches:

```json
{
  "sprint_id": "SP-10006",
  "task_id": "T-10005",
  "category_id": "C-10001",
  "project_id": "P-10000",
  "subtask_id": null,
  "status": "Open",
  "runnable": true
}
```

A matching name in the wrong Category or Project is not a match.

## Current source authority

Use the runtime projection first:

```text
GET /api/v1/projects/{projectID}/execution-tree
GET /api/v1/projects/{projectID}/operational-summary
GET /api/v1/projects/{projectID}/attention
```

Use `internal/model/` and `internal/store/` to understand fields and validation. Use this document to interpret the domain, not to override runtime state.
