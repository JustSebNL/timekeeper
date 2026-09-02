---
name: timekeeper-agent-harness
description: Use when an AI coder must plan and execute long-horizon work through TimeKeeper.
---

# TimeKeeper agent harness

TimeKeeper is the durable authority for this project's work. Chat context is temporary; Project, Task, Sprint, note, event, and usage records are the continuity layer.

## Session start

1. Run `.timekeeper/app/tk doctor` or `tk doctor`.
2. If unavailable, run `bash scripts/timekeeper-harness.sh` (Windows: `scripts/timekeeper-harness.bat`).
3. Inspect `tk list`, then `tk tree <project-id>` and `tk summary <project-id>`.
4. Load the relevant local Skill under `harness/skills/`.

## Execution loop

1. Create or identify a Project and landscape.
2. Break work into Categories, Tasks, Subtasks, and 15–45 minute Sprints.
3. Start one Sprint for the concrete next action.
4. Record notes for decisions, discoveries, blockers, and handoffs.
5. Verify behavior, then complete the Sprint and read back events.
6. On context loss, resume from the tree, summary, notes, and events—not memory guesses.

## Rules

- Never claim completion from model text alone.
- Never store credentials, prompt bodies, or private transcript content in TimeKeeper.
- Do not create duplicate supervisors or continuous health pollers.
- Use `On Hold` with a reason for real blockers.
- Use `TimedOut` after bounded failed retrieval attempts.
- Token usage is cumulative input to TimeKeeper; repeated session/turn snapshots must be idempotent.
