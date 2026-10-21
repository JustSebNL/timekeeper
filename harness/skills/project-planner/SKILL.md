---
name: timekeeper-project-planner
description: Use when translating an outcome or backlog into a TimeKeeper Project, Category, Task, Subtask, and Sprint plan.
---

# Project planner

Plan the smallest durable hierarchy that makes the next work executable. TimeKeeper is the source of truth; the agent proposes and applies ordinary safe changes through `tk`, then reads them back.

## Start with authority and context

1. Read `harness/SKILL.md` and `docs/queue-logic.md`.
2. Run `tk doctor`, `tk list`, then inspect the owning Project with `tk tree <project-id>` and `tk summary <project-id>`.
3. Resolve the implementation-owning Project, not the Project used merely to track another product. Do not create a duplicate Project because a name feels unfamiliar; compare goal, description, and existing categories first.
4. Load current work and status before adding anything. The friendly contact point is `http://timekeeper.local/`; CLI uses it by default and falls back to loopback if necessary.

## Build the hierarchy

```text
Project -> Category -> Task -> optional Subtask -> Sprint
```

- Project: an enduring product/work ownership boundary with a distinct objective.
- Category: a durable functional area inside one Project; choose the narrowest existing fit.
- Task: an independently verifiable outcome, not a single edit, command, or chat turn.
- Subtask: an optional durable subdivision beneath an existing Task when it needs its own direct Sprint plan; do not add one just to make a hierarchy look complete.
- Sprint: one bounded, executable, verifiable slice. Start with a single next Sprint, not a speculative backlog of every possible step.

A Task remains in the Task list while its Sprints execute. A Task or Subtask can own at most four direct Sprints; count only its own direct Sprints. Completed, TimedOut, and Cancelled Sprints remain history and still occupy capacity. See `docs/queue-logic.md` for exact queue and lifecycle rules.

## Task versus Subtask versus new Task

Continue an existing Task when its goal owns the same durable outcome and the new work is a next step, correction, verification, or bounded continuation. Create a Subtask only when the existing Task is correct but the work is a meaningful subdivision with a separate owner and Sprint set. Create a new Task when the outcome is independently verifiable and no existing Task safely owns it, or when the old Task is a completed delivery boundary. Never reopen completed history to attach unrelated work.

## Estimates and sequencing

Use a positive bounded estimate for each Sprint. Split broad work into independently verifiable Sprints rather than assigning one oversized Sprint. Respect parent status and Sprint capacity. Do not mark a Task complete while it has non-terminal Sprints or Subtasks. A Sprint must be Active before it can be completed.

## Apply and log

1. Reuse matching open Categories/Tasks/Subtasks. Create only the missing layer.
2. Create the next Sprint under a Task or Subtask, never directly under a Category.
3. Start the Sprint before work. Do not use `sp next` blindly when an identified Sprint is the intended target; it claims the oldest runnable Sprint, which may be unrelated.
4. Log planning decisions and material evidence in the owning Project as they happen:

```bash
tk msg <project-id> add --kind decision "<ownership choice and why>"
tk msg <project-id> add --kind note "<plan, dependency, or handoff>"
tk sp start <sprint-id> "<bounded objective>"
```

5. Verify creations by reading back `tk tree <project-id>` and `tk summary <project-id>`. Complete only after actual work and verification; otherwise leave active with progress or put it On Hold with a reason.

Never create a second Core/database to get around an existing instance. If duplicate-instance protection refuses startup, diagnose the service and database target under `harness/SKILL.md`.
