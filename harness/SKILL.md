---
name: timekeeper-agent-harness
description: Use when an AI coder must plan and execute long-horizon work through TimeKeeper.
---

# TimeKeeper agent harness

This file is the compact operational contract for agents using TimeKeeper.
For the first-use walkthrough, read [`../docs/HARNESS.md`](../docs/HARNESS.md).

## Session start

1. Run `.timekeeper/app/tk doctor` or `tk doctor`.
2. If the service is unavailable, follow the failure-only recovery procedure below.
3. Inspect `tk list`, then `tk tree <project-id>` and `tk summary <project-id>`.
4. Load the relevant local Skill under `harness/skills/`.

## Failure-only health recovery

The agent workflow does not need a separate harness executable. After `tk doctor`
reports that TimeKeeper is unavailable, invoke the platform service kick once:

- Linux/WSL: `bash scripts/service/kick-server.sh`
- Windows: `scripts\\service\\kick-server.bat`

Then run `tk doctor` again. The kick is a bounded recovery action, not a timer or
continuous poller. If the second health check fails, inspect `.timekeeper/log/`
and preserve the blocker in the current Task/Sprint record. Do not create a
second server or silently continue without durable TimeKeeper state.

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
