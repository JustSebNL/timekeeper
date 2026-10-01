# Jev KB — Choosing the Correct Project

## Objective

Select the single Project that owns the work intent. Project selection happens before Category, Task, or Sprint selection.

## Selection algorithm

1. If the caller supplied a canonical `P-...` ID, read it and verify it exists.
2. If the caller supplied an exact Project alias, resolve the alias and verify uniqueness.
3. If the caller supplied an exact Project name, list matching Projects and continue only when exactly one active candidate exists.
4. If there are zero matches, return `project_not_found` or request clarification.
5. If there are multiple plausible matches, return `needs_review`; never pick the first result.
6. Read the Project's goal, description, status, and execution tree.
7. Compare the intent with Project scope, not only Project name.
8. Reject Completed, Cancelled, or On Hold Projects for new automatic work unless an explicit resume policy permits it.
9. Preserve the selected canonical ID in every later request.

## Evidence used for Project matching

Use, in order:

```text
explicit caller project reference
exact alias/name match
Project goal
Project description
existing Category and Task vocabulary
recent Project evidence/checkpoints
active agent/session binding
```

Recent evidence supports a decision but does not override an explicit project reference or status policy.

## What is not evidence

Do not select a Project because:

- its name shares one noun with the intent;
- it is the oldest Project;
- it has the most recent activity;
- it has the most open Sprints;
- Jev remembers a previous unrelated session;
- a child Category has a similar name but belongs elsewhere.

## Project candidate representation

```json
{
  "key": "P-10000",
  "name": "Suck-It-Up",
  "alias": "suck-it-up",
  "goal": "Download queue and task-container execution",
  "description": "Owns Download-tab task containers, Sprint lanes, and drag/drop behavior.",
  "status": "Open",
  "category_count": 4,
  "open_task_count": 7,
  "active_sprint_count": 1
}
```

Do not send full transcripts, source trees, credentials, or unbounded logs.

## Project boundary examples

### Clear selection

Intent:

```text
Fix the Download tab so Task containers remain in the Tasks queue.
```

Candidates:

```text
Suck-It-Up: owns Download tab, Task containers, Sprint lanes
TimeKeeper: owns the project tracker itself
```

Select Suck-It-Up because the intent concerns its product behavior. TimeKeeper may track the work, but its product source is not the owner.

### Similar names, ambiguous scope

Intent:

```text
Fix the Guardian acknowledgement race.
```

Candidates:

```text
TimeKeeper: owns Pulse Guardian and acknowledgement
Hercules: owns agent orchestration
```

Select TimeKeeper only if the intent concerns TimeKeeper's Guardian code. If the intent is about Hercules' orchestration adapter, select Hercules. Ask for clarification when the wording does not establish ownership.

### Cross-project contamination

A Suck-It-Up implementation defect must not be filed under the TimeKeeper product Project merely because TimeKeeper is used to track it. Tracking scope and implementation ownership are separate.

## Output

```json
{
  "action": "select_project | needs_review",
  "project_id": "P-10000",
  "confidence": 0.91,
  "evidence": ["exact alias", "Project goal matches intent"],
  "reason": "..."
}
```

`project_id` must be absent when the decision is `needs_review` unless it is merely the explicitly requested scope awaiting confirmation.

## Safety rule

If Project ownership is uncertain, stop before Category selection. A confident Category decision inside the wrong Project is still a wrong decision.
