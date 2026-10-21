# Task Workspace Implementation Plan

## Outcome

Implement the approved Task/Sprint workspace slice in `T-11497`, keeping Core as the only authority for data and lifecycle rules. The Task can be opened from the Project hierarchy, inspected with its direct Subtasks/Sprints, and updated through the existing supported Core APIs. A selected Sprint is inspectable and offers only transitions allowed by Core. All successful writes refresh authoritative Task detail and Project hierarchy.

## Existing constraints and contracts

- Approved design: `docs/superpowers/specs/2026-10-10-task-workspace-design.md` (commit `ff96902`); user approval of the refactor includes ordinary implementation details (`M-1319`).
- Existing routes: `GET /api/v1/tasks/{taskID}`, `POST /api/v1/tasks/{taskID}/status`, and `POST /api/v1/sprints/{sprintID}/{start|hold|resume|complete|cancel}`.
- Task statuses offered: `Open`, `On Hold`, `Completed`, `Cancelled`. Task status updates send an explicit status.
- Sprint transitions come from `internal/lifecycle.SprintTransition`; hold and cancel collect a nonblank reason. Terminal `Completed`, `Cancelled`, and `TimedOut` Sprints are read-only.
- Do not add persistence, route handlers, domain transitions, automatic starts, or a second Core. The UI may explain Core-provided blockers but must not authorize writes locally.
- Preserve unrelated dirty worktree changes.

## Slice 1 — Core projection and typed Wails boundary

Files: `internal/api/server.go`, `internal/api/task_detail_test.go`, `internal/desktop/core.go`, new/updated `internal/desktop/*_test.go`, `cmd/desktop/app.go`, `cmd/desktop/workspace.go`, `cmd/desktop/*_test.go`.

1. Add a failing API regression proving a Task whose only Sprint is `TimedOut` reports `children.open_sprints == 0` and `children.can_complete == true`; keep the existing write-guard test as the matching authoritative behavior.
2. Run that focused test and observe failure, then align `summariseChildren` with the Store's terminal Sprint states (`Completed`, `Cancelled`, `TimedOut`).
3. Add typed Task detail response and bounded Core client reads for Task detail; test URL escaping, empty-ID rejection, malformed/oversized responses as appropriate, and status-only non-success errors.
4. Add bounded Core client mutations for Task status and Sprint action. Send only the supported payload shape; validate identifiers/action allowlist and return status-only errors without reflecting arbitrary Core bodies.
5. Expose the methods through `DesktopApp` with the same request deadline as existing bindings. Test delegation and payloads against `httptest` Core endpoints.
6. Run `go test ./internal/api ./internal/desktop ./cmd/desktop ./internal/store`.

Acceptance: Task detail projection and write guard agree for timed-out work; Go Wails/Core boundary has typed tested reads/writes and does not access Store directly.

## Slice 2 — Task and Sprint frontend workspace

Files: `frontend/src/lib/api.ts`, `frontend/src/lib/api.test.ts`, `frontend/src/lib/ProjectWorkspace.svelte`, `frontend/src/lib/ProjectWorkspace.test.ts`, new `frontend/src/lib/TaskWorkspace.svelte` and tests, and `frontend/src/App.svelte` plus tests where needed.

1. Extend API types for Task detail, Subtask, Sprint, child-completion summary, Task status update, and supported Sprint actions.
2. Add frontend tests first for HTTP fallback routes/payloads, desktop binding selection, safe non-success handling, and refresh-oriented method results.
3. Make Task rows keyboard-accessible controls while leaving other hierarchy rows informational. Preserve project context and add an explicit return-to-hierarchy path.
4. Build a focused Task workspace with loading/error/not-found states, Task metadata and children, and nested Sprint selection/details. Display recorded durations as recorded totals, not as a live timer.
5. Derive visible Sprint actions from the exact status/action matrix; require reason entry for hold/cancel, confirmation for Task cancel, and explain `can_complete == false`. Disable duplicate submission while a request is in flight.
6. On mutation success reload Task detail and Project hierarchy; on failure retain current view, show bounded actionable error state, and offer retry/refresh.
7. Verify keyboard/focus and accessible names. The initial Node/SSR setup could not exercise component interaction; after independent review identified missing coverage, the slice added `@testing-library/svelte` and `jsdom` as dev dependencies and configured Vite's Svelte testing plugin. This enables actual browser-like interaction checks without changing runtime dependencies.

Acceptance: typed UI uses the Wails bindings in desktop and only the existing Core routes in browser fallback; no unsupported action is offered and successful writes cannot appear successful until refreshed from Core.

## Slice 3 — Integration and verification

Files: affected frontend and Go tests only unless a concrete regression requires another scoped change.

1. Add/adjust App-level tests proving selection, back navigation, Task detail loading, Sprint selection and refresh integration.
2. Run frontend checks: `npm test`, `npm run check`, `npm run build` from `frontend/`.
3. Run focused Go tests from repository root: `go test ./internal/api ./internal/desktop ./cmd/desktop ./internal/store`; run `go vet ./internal/api ./internal/desktop ./cmd/desktop`.
4. Build the supported Windows desktop package using the repository's existing Wails build command; inspect `cmd/desktop` documentation/scripts first and report whether this proves compilation/package contents versus GUI runtime behavior.
5. Review the final diff for accidental scope expansion, arbitrary server-body rendering, unsupported lifecycle actions, unrelated edits, and tests that merely assert static markup.
6. Record exact commands, results, files, and remaining limits in the owning TimeKeeper Sprint checkpoint. Complete only verified slices; leave any unfinished Sprint held with a precise next action.

## Execution order

Run one mapped Sprint at a time: Core contract/bindings, frontend workspace, integration/verification. Keep `T-11497` open until all acceptance criteria in the approved spec are verified.
