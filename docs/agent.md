# TimeKeeper Agent Harness Router

This document is the detailed routing guide for choosing and combining the project-local harnesses while working in a TimeKeeper-enabled repository. It does not replace the operational contract in [`../harness/SKILL.md`](../harness/SKILL.md), the concise first-use guide in [`HARNESS.md`](HARNESS.md), repository-specific instructions, or the individual Skills. Read those first; then load the Skill(s) matching the actual work.

## Authority and precedence

Use this order when deciding what to do:

1. The user's current request, constraints, and approval boundaries.
2. The target repository's `AGENTS.md`, `TIMEKEEPER.md`, README, and current project brief.
3. TimeKeeper's live Project tree, Task/Sprint state, APIs, and Store validation.
4. The operational contract in [`../harness/SKILL.md`](../harness/SKILL.md).
5. The role-specific harness/Skills listed below.
6. Prior conversation or model assumptions.

A harness is workflow guidance, not new authority. It cannot authorize deployment, secret use, spending, destructive data operations, or work outside the requested scope. Do not infer project state from old conversation when live state is available.

## Start every project work session

1. Read repository instructions and the relevant project brief.
2. Resolve the source-owning Project; tracking a project in TimeKeeper does not make TimeKeeper the owner of that source code.
3. Run `tk doctor`, then inspect `tk list`, `tk tree <project-id>`, and `tk summary <project-id>` before creating duplicate work.
4. Load one primary role Skill from the router below. Load supporting Skills only when the task actually crosses those functions.
5. Choose or create the correct Task and one bounded Sprint. Start it before execution; do not claim a generic queue item blindly when a specific Sprint is the intended target.
6. Store verified reusable gem-mining results in the owning Project's Treasure Chest; use Trail of Thought for decisions, blockers, progress, and verification checkpoints.
7. Verify and read back the exact TimeKeeper state. Do not treat a successful CLI exit or model summary as proof.

Use `http://timekeeper.local/` as the normal entry point for agents and users. The CLI selects it by default; if it is unavailable, diagnose with `tk doctor` rather than substituting another address in instructions. The VS Code companion has a separate origin-allowlist requirement; follow its own documentation.

If TimeKeeper reports an existing instance, follow the failure-only recovery procedure. Do not start a second Core or database to work around an unavailable service; never delete the persistent `.instance.lock` file to clear a lock.

## Inspect the bundled harness from the CLI

The harness is included with the local source/installed app bundle and can be retrieved without the HTTP service:

```bash
tk harness list
tk harness show autonomous-coder
tk harness show deep-audit-fixer
tk harness show project-planner
```

`show` accepts an identifier printed by `list` or the exact listed relative path. It refuses unlisted paths and traversal attempts; it reads bundled files only. `list` prints direct HTML-page links such as `http://timekeeper.local/harness.html?item=autonomous-coder`. Opening one shows the dynamic directory-driven menu and loads the selected Markdown/YAML file into the textarea; file contents remain source text, not HTML-rendered Markdown. `tk harness show agent-router` retrieves the exact file contents in a terminal.

## Choose the primary harness by function

| The work is… | Primary Skill | Use it to… |
|---|---|---|
| A first interaction or learning the local workflow | [`onboarding`](../harness/skills/onboarding/SKILL.md) | Confirm service, discover existing work, make one small useful change. |
| Continuing an already authorized project toward a defined finish | [`autonomous-coder`](../harness/skills/autonomous-coder/SKILL.md) | Keep selecting bounded work until the agreed acceptance criteria are verified, or a real blocker/approval boundary remains. |
| Shaping a goal, backlog, or project landscape into durable work | [`project-planner`](../harness/skills/project-planner/SKILL.md) | Select the owning Project/Category and create the smallest useful Task/Subtask/Sprint plan. |
| Finding whether a reported defect is real and where it breaks | [`bug-hunter`](../harness/skills/bug-hunter/SKILL.md) | Reproduce, trace, and report evidence without silently mixing in a fix. |
| Fixing a confirmed, bounded defect | [`bug-fixer`](../harness/skills/bug-fixer/SKILL.md) | Add/verify a regression, make one narrow root-cause change, and prove it. |
| Auditing and repairing an entire first-party application | [`deep-audit-fixer`](../harness/skills/deep-audit-fixer/SKILL.md) | Track full file/function coverage, trace call paths and wiring, fix confirmed bugs, and keep going until the agreed audit scope is complete. |
| Studying external codebases for reusable patterns | [`gem-mining`](../harness/skills/gem-mining/SKILL.md) | Read selected source repos open-mindedly and completely, store verified gems in the target Project's Treasure Chest as they surface; use ToTb for the general handoff. |
| Independently checking a completed change | [`code-reviewer`](../harness/skills/code-reviewer/SKILL.md) | Review correctness, regressions, and evidence; return findings to the implementer. |
| Importing TODOs/FIXMEs or issue lists | [`backlog-sync`](../harness/skills/backlog-sync/SKILL.md) | Verify each item against current source and import each real outcome once. |
| Reviewing an unfamiliar codebase without a specific defect | [`codebase-audit`](../harness/skills/codebase-audit/SKILL.md) | Inventory architecture, trace important paths, and separate hypotheses from confirmed bugs. |
| Estimating and executing an individual bounded Sprint | [`timeboxing`](../harness/skills/timeboxing/SKILL.md) | Keep the active slice honest, adjust at real boundaries, and record blockers. |
| Recovering service availability | [`service-recovery`](../harness/skills/service-recovery/SKILL.md) | Run health-first, failure-only recovery without spawning another server. |
| Diagnosing a broken workflow or command | [`troubleshooting`](../harness/skills/troubleshooting/SKILL.md) | Gather evidence, isolate the layer, and make a narrow verified correction. |

## Combine roles without blurring their jobs

Use sequential handoffs for distinct functions:

- **Plan then build:** `project-planner` defines ownership and bounded work; `autonomous-coder` executes the approved scope.
- **Investigate then repair:** `bug-hunter` establishes reproduction/root cause; `bug-fixer` implements the confirmed fix.
- **Full audit and repair:** `deep-audit-fixer` includes bug-hunting and fixing phases but maintains a project-wide coverage ledger and reports incomplete coverage honestly.
- **Mine then implement:** `gem-mining` produces source-backed findings first. Implementation is a separate step in the target Project, usually under `autonomous-coder` or `bug-fixer`.
- **Implement then review:** the implementer verifies its change; `code-reviewer` independently inspects it and reports actionable findings. The reviewer does not complete the implementer's Sprint.

A supporting Skill may be loaded alongside the primary one, but never let two role instructions create competing owners, duplicate Sprints, overlapping edits, or double logging for one event.

## What “continue until finished” means

The autonomous roles do not stop at an arbitrary small milestone while approved, safe, runnable work remains. They continue by refreshing repository and TimeKeeper state, selecting the next authorized outcome, executing one bounded Sprint, verifying it, recording evidence, and repeating.

“Finished” is defined by the requested/project acceptance criteria, not by an endless wishlist. A real approval gate, missing user decision, unavailable credential, destructive action, external deployment, or external dependency is a genuine blocker. Record it, hold only the affected Sprint, and keep doing independent safe work where possible. If a context/run limit forces an end, checkpoint the precise next action and resume later; do not claim project completion at that boundary.

For the deep audit/fixer role, “complete audit” additionally requires reconciling the coverage ledger against the agreed first-party file inventory and tracing every function/call path in scope. For gem mining, completion requires the agreed source read coverage and a useful evidence-backed handoff; each verified reusable pattern belongs in the target Project's Treasure Chest (`tk tc`), while Trail of Thought remains for progress, decisions, and handoff pointers. Finding one clever function is not a full mine.

## Durable TimeKeeper logging contract

Log in the Project that owns the source work. Record important progress as it happens, not as one end-of-session report. Keep messages concise and below the 1000-Unicode-character body limit; do not log credentials, private prompts, or unbounded raw logs. Read back returned M-refs before relying on them.

Typical commands:

```bash
tk msg <project-id> add --kind decision "<decision and rationale>"
tk msg <project-id> add --kind observation --tags <topic> "<evidence-backed finding>"
tk msg <project-id> add --kind note "<blocker, handoff, or exact next action>"
tk msg <project-id> show <M-id>
tk sp start <sprint-id> "<bounded objective>"
tk sp complete <sprint-id> "<verified result and actual checks>"
```

A completed Sprint needs actual work, verification evidence, and state readback. A blocked Sprint is `On Hold` with a real reason. Do not complete Tasks/Projects just to make the tree look green; follow `docs/queue-logic.md` and the live Store rules.

## Harness inventory and maintenance

The machine-readable inventory is [`../harness/manifest.yaml`](../harness/manifest.yaml). The short workflow entrypoint is [`../harness/SKILL.md`](../harness/SKILL.md); the step-by-step agent quick start is [`HARNESS.md`](HARNESS.md); role-specific procedures live in [`../harness/skills/`](../harness/skills/).

When a role or contact-point contract changes, update the Skill, manifest, this router, the first-use guide, and relevant README/CLI references together. Check that every manifest Skill path and relative Markdown link resolves, then run `git diff --check`. For code changes, run tests/builds too. Source verification does not update the installed service; report that boundary explicitly.
