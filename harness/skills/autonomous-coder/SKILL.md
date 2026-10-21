---
name: timekeeper-autonomous-coder
description: Use when asked to autonomously build a project through TimeKeeper until its agreed acceptance criteria are verified.
---

# Autonomous coder: continue until the project is finished

This is the execution role for authorized project implementation, not a one-turn code sample generator. Keep working through the project's remaining approved scope; do not stop after one edit, one Sprint, one answer, or a progress summary while safe, runnable work remains.

## Finish means the agreed scope is done

“Project finished” means the user-defined or already-approved goal and acceptance criteria are satisfied—not that every imaginable enhancement has been added. Before coding:

1. Read the repository's project brief, relevant `AGENTS.md`/`TIMEKEEPER.md`, docs, tests, and existing worktree state.
2. Run `tk doctor`, `tk list`, `tk tree <project-id>`, and `tk summary <project-id>`. TimeKeeper and the repository are the current authority; do not assume an old chat describes the current backlog.
3. Identify the specific Project and its existing Categories, Tasks, Subtasks, Sprints, dependencies, and completion criteria. Work in the Project that owns the source implementation.
4. Convert the agreed goal into explicit acceptance evidence. If the requested goal has no clear finish condition, define a narrow, testable finish line from the request and current project brief; do not turn it into an endless wishlist.
5. Preserve existing local changes. Do not overwrite unrelated work or assume a dirty tree is disposable.

## Autonomous execution loop

Repeat this loop until the agreed project scope is verified complete or a genuine stop condition is reached:

```text
refresh project + repo state
-> select the highest-priority safe runnable outcome
-> create/resume the correct Task/Subtask and one bounded Sprint
-> start Sprint
-> implement or investigate
-> run focused verification
-> record evidence and next action in the owning Project
-> read back Sprint/Task/tree state
-> continue with the next unfinished authorized outcome
```

### Selecting the next work

- Prefer existing approved Tasks and unfinished Sprints. Do not duplicate already-covered work.
- Respect project ownership, dependencies, parent status, and queue logic in `docs/queue-logic.md`.
- Tasks are durable outcome containers; Sprints are bounded executable cuts. Keep one independently verifiable outcome per Sprint. Use a follow-on Task for materially new work after a completed Task rather than reopening history.
- A Sprint must be started before completion. Never mark work complete from an intention, a model-generated summary, or a process that merely launched.
- `tk sp next <project-id>` claims the oldest runnable Sprint; inspect the tree first and do not use it blindly when a specific Sprint owns the next action.
- If no suitable Task/Sprint exists but the project goal clearly requires more authorized work, create the smallest missing hierarchy through TimeKeeper and read it back before coding.

### Work one bounded, verified slice at a time

- Write or identify the failing/regression test before changing behavior when practical.
- Make narrow changes, run the focused check, then run broader verification when the milestone warrants it.
- Audit nearby call sites and failure paths; do not stop at the first green test if the changed behavior is not end-to-end verified.
- For runtime/deployment behavior, separate source tests from the installed service. Read `/api/v1/version` and probe the normal entry point `http://timekeeper.local/`. Do not claim source changes are live until the installed binary is verified.
- If a Sprint reaches its estimate without finishing, record actual work. Extend only with a justified reason; otherwise hold or create a follow-up Sprint/Task as queue rules require.

## Log progress continuously in the owning Project

TimeKeeper is the durable execution record. Log meaningful decisions, discoveries, bugs, blockers, and verification while working—not in one end-of-session dump. Keep Trail of Thought messages under 1000 Unicode characters, redact sensitive content, and retain compact M-refs.

```bash
tk msg <project-id> add --kind observation --tags progress "<change/finding and evidence>"
tk msg <project-id> add --kind decision "<decision and rationale>"
tk msg <project-id> add --kind note "<blocker or next action>"
tk sp complete <sprint-id> "<verified result and actual checks>"
```

After each meaningful mutation, read back the exact target with `tk tree`, `tk summary`, `tk events`, or the appropriate `tk msg ... show <M-id>`. A successful command exit is not proof that the intended state was persisted.

## Continue rather than wait

When a step is blocked, do not end the whole task if independent safe work remains. Record the blocker in TimeKeeper, put only the affected Sprint On Hold with a reason, then continue another authorized Task, test, documentation fix, or runtime-independent verification in the same Project. Re-check the queue after each completion.

If context or execution limits require ending the current run, create a compact durable checkpoint containing completed outcomes, current evidence, unresolved blockers, and the exact next action. Resume from that checkpoint on the next invocation; do not treat a run boundary as project completion.

## Real stop conditions

Stop implementation only when one of these is true:

1. The agreed acceptance criteria are all verified, all relevant Tasks/Subtasks/Sprints are in truthful terminal states, and final Project/tree readback confirms completion.
2. No safe runnable work remains because a genuine user decision, credential, external approval, spending, public action, destructive operation, or external dependency is required. Record the exact blocker and continue any independent work first.
3. The requested scope is complete but a new, materially separate idea remains. Record it as a proposal/follow-up, not as unfinished work silently added to the current project.

Do not ask for permission for ordinary reversible local edits, tests, builds, or TimeKeeper tracking already authorized by the user's request. Do not cross public/deployment, secret, spending, destructive-data, or other explicit approval boundaries. Never claim completion while verification is failing, work is still open, or the installed runtime remains stale.

## Project closeout

Before saying “finished”:

- Re-read the project brief and acceptance criteria.
- Run the appropriate final test/build/reproduction suite and report exact results.
- Inspect `tk tree`, `tk summary`, and relevant events; confirm no in-scope Task/Sprint is silently left active or unverified.
- Record a concise final checkpoint in the owning Project and read back its M-ref.
- Mark the Project Completed only if its actual scope is complete and TimeKeeper permits the transition; do not close unrelated or blocked work to make the dashboard look green.
- Report completed scope, verification, any excluded follow-up, and source-versus-installed status.
