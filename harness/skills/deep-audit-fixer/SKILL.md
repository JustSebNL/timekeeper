---
name: timekeeper-deep-audit-fixer
description: Use when auditing and repairing a project end-to-end by tracing every first-party function, call path, dependency, and test.
---

# Deep bug hunter and fixer

This is the exhaustive audit-and-repair role. It does not stop after finding a few obvious defects, passing one test, or completing one Sprint. It keeps working through the agreed first-party codebase until coverage is complete and every confirmed, in-scope defect is fixed and verified or explicitly blocked/accepted.

## Definition of done

A project audit is complete only when:

- every first-party source, test, build/config, CLI, API, service, and relevant documentation area in the agreed scope has a recorded coverage status;
- every function in the source scope has been read and its calls, inputs, outputs, error paths, and callers traced far enough to verify that referenced symbols exist and intended behavior is wired end-to-end;
- public entrypoints and integrations (CLI dispatch, API routes, handlers, storage, workers, UI/plugin hooks, installers/service launchers) are checked for missing wiring, dead paths, duplicate paths, and inconsistent contracts;
- every confirmed defect has a reproducible explanation and an outcome: fixed with regression evidence, explicitly deferred/accepted with rationale, or held for a real external/user blocker;
- relevant tests/builds and final state readbacks pass, and the TimeKeeper handoff records the actual coverage and remaining limitations.

Do not claim “the entire project is clean” if files/functions remain unreviewed. Report exact coverage and remaining paths instead.

## 1. Establish scope and tracking before coding

1. Read the repository's `AGENTS.md`, `TIMEKEEPER.md`, architecture/docs index, project README, and this harness contract. Inspect `git status` and preserve unrelated changes.
2. Run `tk doctor`, `tk list`, `tk tree <project-id>`, and `tk summary <project-id>`. Resolve the Project that owns the source code. Do not log another product's implementation as a TimeKeeper defect just because this tracker is handy.
3. Identify the project finish line and audit boundaries: first-party code, tests, configs/build scripts, deployment/service wiring, and user-facing docs. Exclude only external dependencies, `.git`, caches, and binary artifacts that are not authored source; list every excluded directory/category in the coverage ledger.
4. Create or resume one audit Task in the correct Category and a bounded first Sprint. Keep the Task as the durable audit/fix outcome; Sprints are individual coverage/fix/verification cuts. A Task/Subtask owns at most four direct Sprints, so create a justified follow-on Task when capacity is full rather than deleting old Sprints or packing work into one monster Sprint.
5. Write a short baseline note with scope, finish criteria, current branch/runtime version, known existing changes, and the first module to inspect.

## 2. Build a complete coverage map

Inventory files and symbols to prevent omissions, not to decide which code deserves reading. Maintain a durable ledger (in the target repo's `docs/audits/` when the audit is large, plus concise TimeKeeper checkpoints) with:

```text
path | area/module | functions/symbols | callers/callees traced | tests | findings | status
```

Read each in-scope first-party code file from top to bottom, line by line. For every function/method/handler:

1. Verify its definition, imports, exports, receiver/interface implementation, and all referenced symbols in their actual files.
2. Trace callers into the function and calls out of it. Follow the data and control flow across module boundaries; searching a symbol name is navigation only, never proof.
3. Check input validation, state mutation, concurrency/locking, I/O and error propagation, cleanup, logs/diagnostics, and observable outputs.
4. Confirm the function is reachable through its intended entrypoint (route/CLI/worker/event/plugin/installer) and that the entrypoint actually invokes the intended implementation.
5. Read adjacent tests and identify missing behavioral coverage. A function that exists but is never wired, a route never registered, or a documented command that has no implementation is a finding.

For non-text/binary files, record why line-by-line review is impossible and inspect format/metadata or runtime behavior where practical. Never silently mark an unread file as reviewed.

## 3. Audit the complete behavior paths

Work layer-by-layer, and verify both directions of every contract:

- User-facing command/control -> argument parsing -> dispatch -> client/request -> API route -> handler -> store/model -> persisted state -> readback/UI.
- Service install/launch -> arguments/environment -> process ownership -> listener -> health -> logs/recovery.
- Async queue/event/worker -> enqueue -> ownership/claim -> execution -> retry/failure -> acknowledgement -> durable completion.
- UI control -> event binding -> request payload -> endpoint and response shape -> rendered state and error/empty/loading states.

Compare source behavior, tests, docs, help text, and installed runtime independently. Distinguish source defects from stale binary/assets/config. Check for unreferenced/dead code, duplicate implementations, missing error paths, swallowed errors, unsafe defaults, races, path/case/OS differences, stale documentation, and tests that only test a copy rather than shipped code.

Use static analyzers and searches as accelerators for coverage and candidate leads. Every finding must be confirmed by reading the real implementation and, when possible, a test/reproduction. Do not let a checklist, analyzer, or delegated summary replace the line-by-line trace.

## 4. Record and fix findings as work progresses

For each confirmed bug, capture severity, affected file/function, reproduction or source trace, expected/actual behavior, user impact, root cause, regression test, and whether the installed runtime is affected. Keep speculation labeled as a hypothesis until verified.

Log findings and decisions immediately in the source-owning Project:

```bash
tk msg <project-id> add --kind observation --tags audit "<finding, file/function, evidence, severity>"
tk msg <project-id> add --kind decision "<fix/defer choice and rationale>"
tk msg <project-id> add --kind note "<coverage checkpoint and exact next module>"
```

Use compact ToTb bodies under 1000 Unicode characters, redact secrets/private prompts, and read back important M-refs. Keep the detailed ledger under the target project's `docs/audits/` and link it from the Project board when needed; TimeKeeper remains the durable progress/decision record, not a substitute for the full source report.

For each fix:

1. Reproduce the defect or add a failing test first.
2. Make one narrow root-cause change.
3. Run the focused regression, relevant package tests, and broader checks appropriate to the surface.
4. Review nearby call paths and platform variants.
5. Read back relevant TimeKeeper state and record exact test/build/runtime evidence.
6. Continue to the next confirmed finding or uncovered function; do not stop after a single fix while safe in-scope work remains.

## 5. Continue until the project is actually finished

After each module, save coverage and findings in TimeKeeper and update the next action. Resume from that durable checkpoint after context/run limits. If blocked on one path, continue independent safe audit/fix work elsewhere in the same authorized scope. Keep one active bounded Sprint at a time unless the project explicitly uses coordinated isolated workers; do not spawn overlapping workers on the same files.

The autonomous coder may drive this role across all remaining Tasks/Sprints. Do not create a self-perpetuating loop or expand scope into speculative features: finish the agreed project goal and verified findings, then close out truthfully.

## Stop conditions and safety

Continue ordinary local reading, tests, builds, and reversible source edits without repeatedly asking. Stop only when the stated scope is verified complete, no safe runnable work remains due to a genuine blocker/approval boundary, or the user changes scope. Ask before public deployment, spending, credential handling, destructive data changes, or other explicit approval-gated actions. Never kill or replace an unidentified live process.

If the audit concerns the running service, report both source and live state. Use `http://timekeeper.local/` as the normal local entry point, read `/api/v1/version`, verify the relevant service state, and probe changed behavior. A green source suite does not update the installed service.

## Final handoff

Before declaring the project finished, re-read the finish criteria, reconcile the coverage ledger against the file inventory, run the appropriate full test/build gates, inspect `tk tree`, `tk summary`, and events, and verify no in-scope work remains silently active. Record final coverage, fixed/deferred findings, exact checks, runtime deployment status, and any follow-up M-refs in the owning Project. Mark the Project Completed only if the actual scope is complete and TimeKeeper permits it.
