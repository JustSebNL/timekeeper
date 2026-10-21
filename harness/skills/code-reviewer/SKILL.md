---
name: timekeeper-code-reviewer
description: Use when independently reviewing a completed TimeKeeper change for correctness, regressions, and evidence quality.
---

# Code reviewer

Review changes independently. Do not implement fixes while reviewing unless the task explicitly assigns both roles; report actionable findings to the implementation owner.

## Review sequence

1. Read the owning Project/Task/Sprint and acceptance criteria from TimeKeeper. Confirm the intended implementation owner.
2. Inspect the actual diff, surrounding code, tests, and relevant docs. Preserve unrelated working-tree changes.
3. Trace changed behavior from entrypoint through storage/API/runtime. Check failure handling, concurrency, security boundaries, compatibility, and data ownership.
4. Run focused checks and inspect their real output. For server/runtime changes, distinguish source verification from the installed service and its reported version.
5. Classify findings by severity and include exact file/line, reproduction or evidence, impact, and a narrow suggested correction.
6. If no blocker exists, record what was reviewed and the verification performed. Do not state that the live service is updated unless it was independently checked.

## TimeKeeper review logging

Log a concise review note in the owning Project while work is active:

```bash
tk msg <project-id> add --kind observation --tags review "reviewed=<scope>; findings=<P0/P1/P2/P3 or none>; checks=<actual commands/results>; runtime=<source/live status>"
```

A review note is evidence, not permission to complete someone else's Sprint. The assigned implementer owns corrections and final completion.
