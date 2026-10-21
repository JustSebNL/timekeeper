---
name: timekeeper-gem-mining
description: Use when extracting reusable implementation patterns from external repositories for a TimeKeeper-tracked target project.
---

# Gem mining

Gem mining is focused source study, not bulk copying. The goal is a small, evidence-backed handoff of reusable mechanisms that solve a named gap in a target project.

## Source and target boundaries

- Inspiration repositories belong under `/mnt/h/dev/repos/`. Do not mine random active project checkouts as if they were disposable sources.
- Implementation projects live under `/mnt/d/dev/codebase/dev/`. Identify the target and inspect its own architecture, docs, tests, and current gaps before studying sources.
- A gem is a concrete algorithm, data structure, security mitigation, workflow, retry policy, or UX mechanism—not “repo X contains file Y.”
- Read the relevant repo structure and code deeply enough to verify a pattern. Search/README triage helps choose where to read; it is not evidence that a gem works.
- Do not copy a framework wholesale, import its dependencies, or treat “study/mine” as authorization to integrate its runtime, SDK, service, credentials, or network behavior.
- Preserve license awareness. Reimplement the mechanism in the target's conventions; copy expressive code/assets only when explicitly authorized and license-compatible.

## Choose the TimeKeeper record

1. Run `tk doctor`, `tk list`, then inspect the target Project with `tk tree <project-id>` and `tk summary <project-id>`.
2. The Task belongs to the target product/work outcome, not to the inspiration repo and not automatically to the TimeKeeper product Project. TimeKeeper remains the tracking authority.
3. Reuse an existing Task if it owns the target capability gap. Otherwise use the Project Planner Skill to create the right Category/Task. Do not create one Task per source file or one Sprint per repo.
4. Start a bounded Sprint for one mining pass or implementation slice. Keep the candidate list small enough to inspect and verify.

## Mining workflow

1. Define the target gap before looking for solutions.
2. Select a manageable source-repo batch based on concrete relevance, not names alone.
3. For each selected repo, read purpose/architecture, then inspect the files that implement a promising mechanism and its tests.
4. Record a gem only when its behavior and tradeoffs are understood. Keep idea/pattern separate from source-specific assumptions.
5. Produce a focused handoff table, not a full shelf index:

| Source repo / path | Area | Target gap | Why it fits | Suspected native adaptation | Evidence/depth |
|---|---|---|---|---|---|

6. Rank candidates by impact and transplant cost. Explicitly state when a pattern is only an hypothesis or when source/test evidence is incomplete.
7. Only implement after mining is the requested scope. If implementation is authorized, create or resume the target Task/Sprint and use its own tests and architecture.

## Store verified findings: Treasure Chest

The Treasure Chest is the target Project's dedicated, project-scoped store for reusable mechanisms found during gem-hunting. Use it for source-backed gems—not progress notes or general handoffs. Categories and optional subcategories are created by first use, and an attached gem keeps a parent link. Keep each gem focused and evidence-backed, cite the source path/commit and why the pattern matters, stay within Core's 20,000-character content limit, and exclude secrets, private prompts, and raw repository dumps.

```bash
# Add a verified gem; the category/subcategory is created on first use.
tk tc add <target-project-id> "<category>" "<gem-name>" \
  --subcategory "<optional subcategory>" \
  --source "<repo/path>@<commit>#<file>" \
  "<mechanism, evidence, target gap, why it fits, native adaptation>"

# Add related evidence and retrieve the stored gem later.
tk tc attach <target-project-id> <G-id> "<attached-gem-name>" --source "<repo/path>#<file>" "<related evidence>"
tk tc lookup <target-project-id> <query...> [--category "<category>"]
tk tc show <target-project-id> <G-id>
tk tc revisions <target-project-id> <G-id>
```

Before mining, confirm the installed build exposes `tk tc --help`. If it does not, do not use generic `tk msg` as a substitute gem store; retain the verified finding in the target handoff document and note that durable Treasure Chest capture awaits the updated Core binary. Read back each created gem with `tk tc show <project-id> <G-id>` before claiming it is durable.

## Record general working context: Trail of Thought

Trail of Thought (ToTb) is the general Project message board for progress, decisions, blockers, and mining handoffs; it is separate from the Treasure Chest. Use `tk msg` for this working context, not as a gem store. The main harness contract explains ToTb's message-size, context-menu, and context-window behavior.

```bash
# Use ToTb for decisions and mining-pass handoffs, not for the gems themselves.
tk msg <target-project-id> add --kind decision "Shortlist=<...>; deferred=<...>; next=<...>; evidence=<...>"
tk msg <target-project-id> show <M-id>
```

## Sprint tracking

Start the bounded mining Sprint before work and complete it only after the source coverage, gem read-back, and handoff are verified:

```bash
tk sp start <sprint-id> "Mine patterns for <target gap>"
tk sp complete <sprint-id> "<mined sources, verified patterns, handoff evidence>"
```

## Avoid false progress

- Do not claim a repo was deeply mined after reading only its README.
- Do not dump a giant repo index into TimeKeeper or the chat; keep a concise, ranked handoff.
- Do not mark the mining Sprint complete when it merely found candidates but the required handoff or tests are missing.
- Do not start another TimeKeeper Core/database if a running instance is unavailable; follow the failure-only harness recovery and duplicate-instance guard.
- Do not complete a Sprint or Task based on model-generated claims without verified evidence.
