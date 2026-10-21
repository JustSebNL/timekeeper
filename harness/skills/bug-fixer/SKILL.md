---
name: timekeeper-bug-fixer
description: Use when implementing a verified defect fix with a regression test and durable evidence.
---

# Bug fixer

Fix one confirmed root cause at a time. Investigation and implementation are distinct responsibilities; if the report is still speculative, use `bug-hunter` first.

## Before changing code

1. Read `harness/SKILL.md`, `harness/skills/bug-hunter/SKILL.md`, and the relevant project-local Skill.
2. Resolve the implementation-owning Project and inspect its Task/Sprint tree. Reuse the existing defect Task when it owns the fix; do not file the fix under TimeKeeper merely because TimeKeeper tracks it.
3. Read the bug-hunter evidence/checkpoint. Verify the reported failure against current source and current runtime version before relying on it.
4. Start exactly one bounded Sprint for the next implementation slice. Preserve unrelated working-tree changes.

## Fix loop

1. Add or identify a regression test that fails for the confirmed defect.
2. Run it and capture the failing evidence.
3. Make one narrow root-cause change; do not bundle speculative cleanups.
4. Run the focused test, relevant package tests, and then broader verification appropriate to the change.
5. If runtime behavior matters, test the installed service separately from source; never claim deployment from a source test.
6. Read back the relevant state and results. Complete the Sprint only when the fix and its verification are real. If blocked, hold with a specific reason.

## Record while working

Use the owning Project's Trail of Thought board for durable progress and handoffs:

```bash
tk msg <project-id> add --kind observation --tags bug,fix "regression test RED: <test and failure>"
tk msg <project-id> add --kind observation --tags bug,verification "fix: <change>; checks: <real results>; runtime: <source-only or live>"
tk sp complete <sprint-id> "<verified outcome and test evidence>"
```

Do not claim a Sprint complete from an agent's summary alone; inspect the test/build output. Preserve existing M-refs and link related evidence rather than copying long logs. Never place secrets or private prompts in the board.

## Safety boundaries

Do not overwrite the installed database, stop an unknown listener, force-kill an unidentified process, deploy publicly, or make destructive changes without the applicable approval. If an installed Windows service must be replaced, stage and verify first; report explicitly when the live service remains unchanged.
