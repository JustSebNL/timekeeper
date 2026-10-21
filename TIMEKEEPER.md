# TimeKeeper Agent Contract

This project uses TimeKeeper as its durable execution authority. This file is a project pointer, not a replacement for an existing `AGENTS.md`, `CLAUDE.md`, or equivalent agent instruction file.

## First use

For a new agent, start with the [onboarding Skill](harness/skills/onboarding/SKILL.md), then use the [Agent Harness Router](docs/agent.md) to choose the role procedure and follow [`docs/HARNESS.md`](docs/HARNESS.md) plus the operational `harness/SKILL.md` contract. If the server executable is not on `PATH`, use `.timekeeper/app/timekeeper --help` on Linux/WSL or `.timekeeper/app/bin/timekeeper.exe --help` on Windows.

1. Run `tk doctor`; the advertised/default contact is `http://timekeeper.local/` (the CLI uses loopback only as a fallback).
2. Inspect or create the Project landscape before changing code.
3. Work from explicit Project, Category, Task, and Sprint IDs.
4. Use bounded Sprint estimates. At most three extensions are allowed, totaling no more than the original estimate. Hold and reassess exhausted work; do not evade limits with equivalent replacement Sprints.
5. Record blockers, decisions, discoveries, and handoffs as Project notes/events.
6. Verify changes with tests or an explicit evidence step before completing a Sprint.
7. Read the tree, summary, and events when resuming after a context window or agent handoff.

Load the relevant project-local Skill from `harness/skills/` before planning, auditing, timeboxing, or troubleshooting. The [autonomous coder Skill](harness/skills/autonomous-coder/SKILL.md) is the execution loop for continuing authorized work until the project's acceptance criteria are verified. The [deep audit/fixer Skill](harness/skills/deep-audit-fixer/SKILL.md) exhaustively traces first-party functions and fixes verified defects while recording coverage in TimeKeeper. Role-specific workflows are indexed in [`docs/HARNESS.md`](docs/HARNESS.md): planner, bug hunter, bug fixer, gem miner, and code reviewer. Each role records progress in the source-owning TimeKeeper Project while work is active.

## Recovery

If TimeKeeper is unavailable, follow the failure-only recovery procedure in
[`harness/SKILL.md`](harness/SKILL.md). It checks `/health`, invokes the
platform service kick only after a failed check, and then checks health again.
Do not bypass the durable task record by silently tracking work only in chat. Legacy wrapper paths `scripts/timekeeper-harness.sh` and `scripts/timekeeper-harness.bat` remain only for existing integrations; new agents should follow `harness/SKILL.md` directly.

## Truth boundary

TimeKeeper records explicit project execution state. It does not prove model quality, token cost, or task completion from agent claims alone. Unknown usage/pricing remains unknown; secrets and private prompt bodies do not belong in TimeKeeper records.

For current Guardian deadline behavior, usage attribution, and known runtime integration limits, read [Execution safeguards](docs/execution-safeguards.md).
