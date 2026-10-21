---
name: timekeeper-bug-hunter
description: Use when investigating a suspected defect and separating verified facts from hypotheses.
---

# Bug hunter

The mission is to establish whether a defect exists, reproduce it, isolate its failing layer, and leave a usable evidence trail. Do not patch code in this role unless the user explicitly assigns both investigation and repair.

## Intake and ownership

1. Read `harness/SKILL.md` and `harness/skills/codebase-audit/SKILL.md`.
2. Run `tk doctor`; use the primary `http://timekeeper.local/` contact through the CLI. Inspect the owning Project with `tk list`, `tk tree`, and `tk summary` before creating work.
3. Identify the product/source owner. A bug in another application belongs to that application's Project, even if TimeKeeper is used to track it.
4. Reuse an existing open Task/Sprint if it owns this defect. Otherwise ask the planner workflow to create the smallest durable Task/Sprint; avoid duplicate Tasks for repeated reports of the same root cause.

## Evidence-first investigation

- Capture exact expected versus actual behavior, environment/version, reproduction steps, and affected paths.
- Reproduce with a focused test or deterministic command. Read the complete error, trace inputs across component boundaries, inspect logs and current source.
- Separate confirmed facts, hypotheses, and unknowns. Do not turn a plausible theory into a confirmed Task description.
- Distinguish source failure from stale installed/runtime behavior. Check the running `/api/v1/version`, service/listener ownership, and route/asset source separately.
- Keep the investigation Sprint bounded: reproduce, trace, identify the failing layer, and produce a diagnosis. Split repair into a follow-on Sprint when the fix needs its own work and verification.
- Stop before destructive cleanup, killing an unidentified process, external publication, or changing credentials.

## Record findings as you learn them

Post one compact observation/decision in the owning Project board while investigating, not as a giant end-of-run dump:

```bash
tk msg <project-id> add --kind observation --tags bug,reproduction "expected=... actual=... repro=... version=... evidence=..."
tk msg <project-id> add --kind decision "confirmed root cause: ...; failing layer: ...; next action: ..."
```

Keep posts below the board's 1000-Unicode-character limit. Reference source paths, test names, event IDs, or M-refs. Do not store secrets, private prompts, or unbounded logs.

## Handoff

The diagnosis handoff must say:

- whether the bug was reproduced;
- the exact failing layer and evidence;
- confirmed root cause versus remaining hypotheses;
- a narrow proposed fix and regression test;
- whether the evidence came from source or the installed runtime;
- the next Sprint/Task and verified state.

Do not complete the investigation Sprint until the evidence is read back and the finding is recorded. If blocked, use `On Hold` with the actual blocker; do not leave a false completion claim.
