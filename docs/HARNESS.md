# Agent first use: TimeKeeper harness

This guide is for an AI coding agent working in a TimeKeeper-enabled repository. Human users who want to install or try TimeKeeper should start with the [README](../README.md) and [install guide](install.md).

Use `tk harness list` or open the [Harness Viewer](http://timekeeper.local/harness.html) to inspect the available documents. The viewer builds its menu at runtime from supported `.md`, `.yaml`, and `.yml` files under `harness/`; selecting an item loads the source into a read-only textarea with a **Copy file** button. Adding, removing, renaming, or editing a harness file updates the menu/content without changing the viewer HTML. The manifest remains validated as the machine-readable agent contract. Individual `http://timekeeper.local/harness.html?item=<id>` links open the HTML viewer with that item selected, while `tk harness show <listed-item>` prints the exact source in the terminal. Use the [Agent Harness Router](agent.md) to choose which item matches the work.

The harness is a workflow guide, not an API manual and not a mechanism that can resume every agent runtime. On first use, begin with the [onboarding Skill](../harness/skills/onboarding/SKILL.md), then read the operational [harness contract](../harness/SKILL.md) and the one project-local Skill relevant to the next action before changing project work.

## 1. Find the local CLI and check the service

The installer does not add `tk` to `PATH`. From the repository root, use the local command if present:

```text
Linux / WSL: ./.timekeeper/app/tk (or ./.timekeeper/app/bin/tk)
Windows:     .\.timekeeper\app\bin\tk.exe
```

Use the resolved CLI for normal work. The standard entry point for people and agents is `http://timekeeper.local/`; the CLI uses this hostname by default. If a command reports an availability problem, run `tk doctor` before attempting recovery.

```bash
tk doctor
```

If the health check fails, follow the one-time, failure-only recovery procedure in [`harness/SKILL.md`](../harness/SKILL.md), then run `tk doctor` once more. Do not start a second server or database, and do not continue project mutations without a healthy durable store.

Current identity-aware CLI builds can resolve a FASE identity automatically. That ID is attribution metadata, not a login or authentication mechanism; see [FASE identity](fase-identity-design.md). An installed binary may not contain source changes that have not been included in its build.

## 2. Inspect existing work

Load only the local Skill relevant to the next action. Available Skills are:

```text
harness/skills/onboarding/SKILL.md
harness/skills/project-landscape/SKILL.md
harness/skills/backlog-sync/SKILL.md
harness/skills/codebase-audit/SKILL.md
harness/skills/timeboxing/SKILL.md
harness/skills/service-recovery/SKILL.md
harness/skills/troubleshooting/SKILL.md
harness/skills/project-planner/SKILL.md
harness/skills/bug-hunter/SKILL.md
harness/skills/bug-fixer/SKILL.md
harness/skills/gem-mining/SKILL.md
harness/skills/code-reviewer/SKILL.md
```

Then inspect the existing project records before creating anything:

```bash
tk list
tk tree <project-id>
tk summary <project-id>
```

Use IDs returned by TimeKeeper exactly. If there is no suitable Project, make the smallest useful structure:

```text
Project -> Category -> Task -> optional Subtask -> Sprint
```

A Sprint is one bounded action, not a vague request to finish a whole project. The included workflow commonly uses 15–45 minute estimates; choose a duration appropriate to the actual work.

## 3. Work, verify, and leave a handoff

Start or resume one concrete Sprint. For gem-mining work, store verified reusable patterns in the Treasure Chest. Use Trail of Thought for progress, decisions, blockers, and handoff pointers; it is the general project context board, not the gem catalogue. Use `On Hold` with a reason for a real blocker instead of silently leaving work active.

Before completing a Sprint, run an appropriate real check: a test, build, reproduction, read-back, or another explicit verification. Record the evidence and inspect the resulting state:

```bash
tk pulse
tk events <project-id>
tk msg <project-id> checkpoint --author <agent-id> \
  --entry 'observation|Test/build/reproduction result.|verification' \
  --entry 'note|Next concrete action.|handoff'
```

After an interruption or handoff, repeat `tk doctor`, `tk tree`, and `tk summary`; read the latest events and checkpoint before continuing. Do not infer current state from an old conversation.

## Role-specific work modes

Load the role Skill that matches the current assignment instead of making every agent follow one generic procedure:

| Role | Skill | Use it for |
|---|---|---|
| Autonomous coder | [`autonomous-coder`](../harness/skills/autonomous-coder/SKILL.md) | Continue authorized project work across bounded Sprints until acceptance evidence is verified. |
| Project planner | [`project-planner`](../harness/skills/project-planner/SKILL.md) | Choose the owning Project/Category and shape Tasks, optional Subtasks, and bounded Sprints. |
| Bug hunter | [`bug-hunter`](../harness/skills/bug-hunter/SKILL.md) | Reproduce a suspected defect, isolate the failing layer, and report evidence without patching. |
| Bug fixer | [`bug-fixer`](../harness/skills/bug-fixer/SKILL.md) | Implement one verified root-cause fix with regression coverage and readback. |
| Deep audit/fixer | [`deep-audit-fixer`](../harness/skills/deep-audit-fixer/SKILL.md) | Dissect the full first-party project, trace each function and integration path, and fix confirmed defects until scoped coverage and acceptance criteria are verified. |
| Gem miner | [`gem-mining`](../harness/skills/gem-mining/SKILL.md) | Study external repos and store verified transferable mechanisms as Treasure Chest gems; use ToTb for the mining decision/handoff. |
| Code reviewer | [`code-reviewer`](../harness/skills/code-reviewer/SKILL.md) | Independently inspect a completed change and record actionable findings. |

Roles are workflow lenses, not new agent processes or permissions. The selected role never overrides the source-owning Project, Sprint lifecycle, approval boundaries, or verification rules.

## Log work inside the owning TimeKeeper Project

Keep tracking attached to the product/source owner. Use the Treasure Chest first for reusable, source-backed gem-mining findings; categories are created by first use. ToTb is the separate general-purpose board for progress, decisions, blockers, and context handoffs. The main [harness contract](../harness/SKILL.md) explains its context-menu and context-window role.

```bash
# Durable gem-hunting artifact.
tk tc add <project-id> <category> <gem-name> <content...> [--subcategory <name>] [--source <repo/path>]
tk tc show <project-id> <G-id>

# General workflow context and verification.
tk msg <project-id> add --kind decision "<choice and rationale>"
tk msg <project-id> add --kind observation --tags <topic> "<reproduction, finding, or verification>"
tk msg <project-id> show <M-id>
```

Before logging, inspect the Project tree and use the actual returned IDs. Keep ToTb message bodies within the 1000-Unicode-character limit, avoid secrets/raw transcripts, and read back important M-refs and G-refs. Start a Sprint before execution, complete it only after real verification, and use On Hold with a reason for blockers.

## Optional agent wake/recovery setup

Ordinary human Project, Task, and Sprint tracking does not require `tk init`, a background worker, or a wake adapter. For agent work following this repository's harness, the manifest includes ASE lease/recovery in its workflow; read the corresponding section below before setting it up. Configure a runtime adapter only when the authorized agent workflow needs unattended attention recovery.

For a compatible runtime, use one stable agent identity, bind the intended Project/Sprint, and configure a trusted adapter explicitly. `tk init --agent <agent-id> --project <project-id> --lease 30m` is an optional local receiver setup; it writes `.tkrc` and registers a lease. It does not authenticate the identity or attach to an arbitrary running conversation. The adapter must be able to resume the intended runtime and meet the verified-continuation contract.

```bash
tk guardian wake --agent <agent-id> --project <project-id> \
  --adapter <trusted-adapter> --poll --interval 30s
```

Renew the lease only at real progress boundaries. A callback being accepted is not proof that work resumed. Acknowledge a nudge only after the adapter reaches its configured continuation boundary and records the outcome. Full details are in [Anti Slack Engine](anti-slack-engine.md) and the [agent wake adapter protocol](agent-wake-adapter-protocol.md).

## Rules worth remembering

- TimeKeeper is the durable project record; chat context is temporary.
- Load the harness contract and the relevant Skill before planning project changes.
- Evidence beats “done” in model text.
- Never store credentials, private prompt bodies, or unbounded logs in TimeKeeper.
- `tk pulse` is a read-only check; it does not send notifications or schedule work.
- Use [Agent/API integration](../AGENT_INTEGRATION.md) for CLI/API and usage-snapshot contracts.
- Use [Execution safeguards](execution-safeguards.md) for limits on supervised Codex runs.

A reliable ordinary-use path is:

```text
check service -> inspect existing work -> choose one bounded Sprint -> work -> verify -> record -> hand off
```
