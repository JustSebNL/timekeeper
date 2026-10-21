---
name: timekeeper-onboarding
description: Use when a person or agent is new to TimeKeeper or needs a gentle first-use walkthrough.
---

# TimeKeeper onboarding

Use this Skill to help someone get comfortable with TimeKeeper before introducing advanced agent or integration workflows. Keep the first session small: confirm the local service, find the existing work, and make one useful next step. The [harness contract](../../SKILL.md) remains authoritative for recovery and agent safety. When the assignment is to continue an authorized project through completion, also load the [autonomous coder Skill](../autonomous-coder/SKILL.md).

## What TimeKeeper does

TimeKeeper keeps a local record of project structure, bounded work, recorded active time, and short notes or handoffs. A Project is organized as:

```text
Project -> Category -> Task -> optional Subtask -> Sprint
```

A Sprint is the next concrete piece of work. Starting, holding, resuming, or completing it records its active-time intervals. The dashboard and CLI use the same local service and SQLite data. TimeKeeper is not a chat archive, cloud-sync workspace, productivity monitor, or independent verifier of work.

## First session

### 1. Find the local CLI and check the service

The installer does not add `tk` to `PATH`. From the repository root, use the local CLI when available:

```text
Linux / WSL: ./.timekeeper/app/tk (or ./.timekeeper/app/bin/tk)
Windows:     .\.timekeeper\app\bin\tk.exe
```

Use `tk` below as shorthand for that resolved command. Check the service once:

```bash
tk doctor
```

If health fails, use only the recovery step in [`harness/SKILL.md`](../../SKILL.md), then check again. Do not start a second server or database. A hint that an optional plugin or history provider is unconfigured does not by itself mean core project tracking is unavailable; act on actual health failures.

### 2. Look for existing work before creating anything

```bash
tk list
tk tree <project-id>
tk summary <project-id>
```

Choose the Project that actually owns the work. Read its latest events or Trail of Thought messages when a handoff or decision matters. Use IDs returned by TimeKeeper; commands do not retain implicit Project context. For a structured plan, load [`project-planner`](../project-planner/SKILL.md); for a suspected issue, separate [`bug-hunter`](../bug-hunter/SKILL.md) diagnosis from [`bug-fixer`](../bug-fixer/SKILL.md) implementation. Use [`deep-audit-fixer`](../deep-audit-fixer/SKILL.md) when the assignment is a complete function-by-function audit and repair of the whole first-party project.

### When several agents share a Project

Several agents can use the same local Project record. Before creating work, each agent should inspect the current tree and Sprint states, use the existing next action where possible, and leave a short handoff for the next worker. A shared record helps coordinate a swarm; it does not provide remote hosting or isolate agents as separate operating-system users. Compatible FASE-aware builds add execution claims, but identity headers and labels are still not authentication.

If there is no suitable Project, make only the smallest structure needed for the next action. For example, after each create command prints its ID:

```text
tk p new "Example Project"
tk c new P-10000 "Delivery"
tk t new P-10000 C-10001 "Update the local setup guide" 1h
tk sp new task T-10002 "Document the Linux install path" 30m
tk sp start SP-10003
```

The IDs above are examples, not IDs to reuse. Replace each one with the ID returned by your own command. Use `http://timekeeper.local/` as the normal contact point for creating and browsing Projects; the CLI uses that hostname by default.

Use `tk harness list` for the live catalog and direct HTML links (e.g. `http://timekeeper.local/harness.html?item=onboarding`). A browser opens the shared HTML viewer with that harness selected in the dynamic menu and its original file contents in a read-only textarea with a **Copy file** button. The viewer discovers supported files under `harness/`; it does not render Markdown as HTML. Use `tk harness show onboarding` to retrieve the original text in the terminal.

### 3. Work, record, and verify

Start one bounded Sprint for the concrete next action. The included workflow commonly uses estimates of 15–45 minutes; use an estimate that fits the work rather than making a Sprint broad enough to cover an entire Project.

While working:

- add a short note or message for a decision, blocker, discovery, or handoff;
- use `On Hold` with a reason when blocked instead of leaving work marked active;
- remember that `tk pulse` is read-only and does not send notifications;
- run a real check before completing work, such as a test, build, reproduction, or read-back.

Example handoff:

```text
tk msg P-10000 add --kind observation "Focused tests pass; next check the Windows path."
```

Record the verification evidence, complete the Sprint only when the work is actually verified, and read the project summary/events back. TimeKeeper preserves what the person or agent records; it does not decide whether the work is correct.

## Pick up after an interruption

Do not reconstruct state from an old chat. Re-check `doctor`, `tree`, and `summary`, then read the latest relevant event or checkpoint. Carry only useful message references such as `M-42` between turns; retrieve the full message when its details are needed.

## Agent identity and optional wake support

Some CLI/Core builds resolve a FASE identity automatically. It is attribution metadata, not a login or authentication mechanism; an `X-Agent-ID` header is not proof of identity. The FASE design reserves each key to exactly one fingerprint and never reassigns it. That reservation prevents key collisions; it does not by itself guarantee that a runtime adapter carries the same fingerprint across a restart. On identity-aware builds, `tk agent identity` returns both `agent_id` and `fase_id`: use the stable `agent_id` (`FASE-<key>`) for `tk guardian wake --agent`; `fase_id` (`FASE-ID.<key>.<month>.<year>`) is the monthly display alias that reuses that key. Source tests cover Guardian resume continuity, but installed-runtime rollout is still unverified; see the [FASE identity status](../../../docs/fase-identity-design.md). Use the installed CLI's `tk help` rather than assuming every installed binary includes the newest source changes.

A normal human dashboard/CLI session does not require a background worker or a generic `tk init` setup. For agent work following this repository's harness, the manifest includes ASE lease/recovery as part of the workflow; read the ASE section in [`harness/SKILL.md`](../../SKILL.md) before using it. Use only a trusted adapter for the chosen runtime, never invent one, and do not claim that a nudge means an agent resumed. If the required runtime adapter is unavailable, follow the harness's blocker path rather than pretending recovery is configured. Do not store credentials, private prompts, or unbounded transcripts in TimeKeeper.

## Where to go next

- [Agent first-use guide](../../../docs/HARNESS.md) — recovery, handoffs, and the full first-use loop.
- [Build a Windows candidate without installing](../../../docs/build-windows-candidate.md) — test and stage updated Windows binaries without changing the service.
- [CLI and API help](../../../HELP.md) — command syntax and examples.
- [Project landscape Skill](../project-landscape/SKILL.md) — set up a new Project structure.
- [Timeboxing Skill](../timeboxing/SKILL.md) — estimate and execute Sprints.
