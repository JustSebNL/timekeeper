# Jev KB — Detecting the Correct Task

## Objective

After Project and Category selection, decide whether the intent belongs to an existing Task, requires a new Task, or must stop for review.

A Task is a substantial outcome inside one Category. It is not a single command, file edit, or chat turn. A Task is also a durable container for one to four direct Sprints.

## Task candidate facts

Each candidate should include:

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
owned direct Sprint summaries
owned Subtask summaries
recent evidence references
```

Example:

```json
{
  "key": "T-10005",
  "kind": "task",
  "project_id": "P-10000",
  "category_id": "C-10001",
  "name": "Keep Task containers in the Tasks queue",
  "goal": "Tasks remain durable containers while their Sprints execute.",
  "status": "Open",
  "direct_sprint_count": 2,
  "direct_sprint_capacity": 2,
  "has_runnable_sprint": true
}
```

Keep direct Task Sprints separate from Subtask-owned Sprints.

## Detecting an existing Task

Prefer an existing Task when all or most of these are true:

1. The Task goal covers the same durable outcome.
2. The Category and Project are exact matches.
3. The Task status permits continuation.
4. An existing Open/Active/On Hold Sprint is related to the intent, or the Task has room for another bounded Sprint.
5. The new intent is a next step, correction, verification, or bounded continuation of the Task rather than a separate outcome.
6. Reusing it will not mix unrelated deliverables.

Use semantic similarity only as evidence. The Task goal, current Sprint names, recent evidence, and parent scope matter more than one shared word.

## When a new Task is appropriate

Propose a new Task when:

```text
No existing Task owns the durable outcome.
The work is independently verifiable.
The work will need one or more bounded Sprints.
The proposed Task belongs to the selected Category.
The proposed Task name and goal are distinct from existing Tasks.
```

Do not create a new Task merely because an existing Task has no currently Open Sprint. A Task can remain the correct durable owner while a new Sprint is proposed, provided it has capacity and its parent chain is Open.

## Task versus Subtask

Choose an existing Subtask when it already owns the narrower outcome and its direct Sprints match the intent.

Propose a new Subtask only when:

- the parent Task is correct;
- the work is a durable subdivision of that Task;
- the subdivision will own its own Sprint(s);
- policy explicitly permits automatic Subtask creation.

If automatic Subtask creation is not enabled, return `needs_review` rather than silently creating a new layer.

## Task status rules

A Task may be Open, On Hold, Completed, or Cancelled. The exact runtime status is authoritative.

Do not route new work automatically to a terminal Task. Do not reopen a Completed or Cancelled Task merely because the intent sounds related. Create a follow-on Task under the same Category when the original Task is a closed delivery boundary and the new work is materially separate.

A Task with non-terminal Sprints or Subtasks cannot be marked Completed. Completing a Sprint does not automatically complete the Task; Task completion is explicit after scope verification.

## Task decision options

```text
existing_task(T-...)
existing_subtask(ST-...)
create_task
create_subtask
none_of_the_above
needs_review
```

The chosen Task/Subtask must always carry its parent Category and Project IDs.

## Typed question template

The Task pass is performed after deterministic Category narrowing:

```json
{
  "id": "task_target",
  "type": "choice",
  "instructions": "Within the selected Category, choose the Task or Subtask that durably owns this intent. Use create_task only when no existing owner fits. Use needs_review when the target or parent scope is uncertain.",
  "criteria": {
    "T-10005": "Task containers remain durable while Sprints execute.",
    "ST-10006": "Queue lane rendering under the selected Task.",
    "create_task": "No existing Task is a safe owner.",
    "needs_review": "Do not create or route automatically."
  }
}
```

A separate Sprint disposition question must not be allowed to select `resume_existing` without a concrete Task/Sprint target.

## Task capacity reasoning

A Task's direct Sprint count is not the same as its total descendant count. Count:

```text
Task direct Sprints where sprint.task_id == task_id and subtask_id is null
```

A Subtask has its own independent direct-Sprint count. Each owner may have up to four direct Sprints.

If the selected Task has four direct Sprints, do not create a fifth. Consider:

```text
resume an existing runnable Sprint
select an existing Subtask with capacity
propose a follow-on Task
route to review
```

Do not delete or overwrite old Sprints to make room.

## Common mistakes

- Choosing a Task from a different Category with a similar name.
- Treating a completed Task as reusable without explicit follow-on policy.
- Creating a new Task for every small implementation step.
- Treating a Task as if it leaves the queue when one Sprint completes.
- Counting Subtask Sprints as direct Task capacity.
- Creating a Sprint under a Category instead of under a Task/Subtask.
- Returning a disposition without a concrete target.

## Output

```json
{
  "action": "existing_task | existing_subtask | create_task | needs_review",
  "project_id": "P-10000",
  "category_id": "C-10001",
  "task_id": "T-10005",
  "subtask_id": null,
  "direct_sprint_count": 2,
  "direct_sprint_capacity": 2,
  "confidence": 0.87,
  "reason": "Existing Task goal and active Sprint scope match the intent.",
  "requires_confirmation": false
}
```
