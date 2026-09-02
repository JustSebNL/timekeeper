# TimeKeeper Agent Contract

This project uses TimeKeeper as its durable execution authority. This file is a project pointer, not a replacement for an existing `AGENTS.md`, `CLAUDE.md`, or equivalent agent instruction file.

## Required workflow

1. Run `tk doctor` against `http://127.0.0.1:1618`.
2. Inspect or create the Project landscape before changing code.
3. Work from explicit Project, Category, Task, and Sprint IDs.
4. Use bounded Sprint estimates; split oversized work instead of silently extending it.
5. Record blockers, decisions, discoveries, and handoffs as Project notes/events.
6. Verify changes with tests or an explicit evidence step before completing a Sprint.
7. Read the tree, summary, and events when resuming after a context window or agent handoff.

Load the relevant project-local Skill from `harness/skills/` before planning, auditing, timeboxing, or troubleshooting.

## Recovery

If TimeKeeper is unavailable, run the failure-only health gate:

```bash
bash scripts/timekeeper-harness.sh
```

The gate checks `/health`, invokes `scripts/service/kick-server.sh` only after a failed check, and retries once. On Windows, use `scripts/timekeeper-harness.bat`. Do not bypass the durable task record by silently tracking work only in chat.

## Truth boundary

TimeKeeper records explicit project execution state. It does not prove model quality, token cost, or task completion from agent claims alone. Unknown usage/pricing remains unknown; secrets and private prompt bodies do not belong in TimeKeeper records.
