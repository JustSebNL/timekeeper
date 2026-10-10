# Task Workspace Design

## Goal

Make a Task and its Sprints a first-class, navigable workspace in the Wails/Svelte UI. The Task view must present authoritative Core state and expose only lifecycle actions that Core already supports. It must remain an adapter over the existing TimeKeeper Core, not a second business-logic or persistence layer.

## Context and current boundary

The Svelte shell now presents Overview and Projects. A Project workspace renders the Core execution hierarchy (`Project → Category → Task → Subtask → Sprint`) and preserves the selected Project across desktop restarts. Task rows are currently read-only and do not open a detail view.

Core already provides `GET /api/v1/tasks/{taskID}`, returning the Task, its direct Subtasks, direct Sprints, and a child summary including `can_complete`. It also exposes Task status updates and Sprint lifecycle transitions. The Wails adapter currently provides typed reads for health, version, projects, and Project execution trees; it does not yet expose Task detail or lifecycle commands.

The migration roadmap's generic action list mentions Start/Pause/Resume/Stop/Complete/Fail/Cancel. That list is not the authoritative domain contract. Current Core Task statuses are `Open`, `On Hold`, `Completed`, and `Cancelled`; current Sprint lifecycle operations are start, hold, resume, complete, and cancel. `TimedOut` is produced by retrieval-attempt policy, not a user action. The UI must not invent `Active`, `Failed`, or unsupported actions for Tasks.

One current contract mismatch must be corrected before the UI gates completion on `children.can_complete`: the Task-detail summary counts `TimedOut` Sprints as open, while `Store.UpdateTaskStatus` treats them as terminal when checking whether a Task may complete. The summary and write guard must agree; a regression test must cover a Task whose only remaining Sprint is `TimedOut`.

## User flow

1. The user opens a Project and sees its execution hierarchy.
2. Selecting a Task row opens a focused Task workspace while retaining the Project context and a clear route back to the hierarchy.
3. The workspace shows Task identity, status, goal/description, priority, calculated completion, estimate, Subtasks, and direct Sprints.
4. Selecting a direct Sprint opens its details within the Task workspace: identity, status, estimate, recorded active/hold durations, hold reason, and start/end timestamps when present. These recorded durations are not a live elapsed timer.
5. The user explicitly requests an available Task status change or Sprint lifecycle transition. The UI reports Core validation failures and refreshes authoritative detail after a successful mutation.

The selected Task is session-local for this first slice. On restart, the existing selected Project restoration returns the user to its hierarchy; Task-specific persistence is deferred.

## Architecture and data flow

- `ProjectWorkspace.svelte` provides a keyboard-accessible Task selection affordance and passes the selected Task ID to a focused `TaskWorkspace.svelte` view. Sprint selection stays scoped beneath that Task.
- The frontend API facade uses typed Task-detail and lifecycle methods. In the desktop runtime those methods call Wails bindings; browser-development fallback, if retained, calls the same existing Core HTTP routes rather than implementing domain rules in JavaScript.
- `DesktopApp` exposes bounded, context-free Wails methods. Each method obtains the existing Core client, applies the existing request deadline, and delegates to typed `CoreClient` methods.
- `CoreClient` validates identifiers, uses bounded HTTP requests, decodes typed API responses, and returns status-only errors for non-success responses. It does not access SQLite or duplicate lifecycle validation.
- Core remains authoritative for legal transitions, completion blockers, agent claims, and durable Project history. The UI may use the returned `can_complete` summary to explain a blocked completion, but the API/store must revalidate every write.
- After a mutation succeeds, the frontend reloads Task detail and the Project hierarchy. It must not optimistically present an unconfirmed status or action result.

## Lifecycle controls

- Task controls may request only the Core-supported statuses: `Open`, `On Hold`, `Completed`, and `Cancelled`. The current Core API treats status as an explicit value rather than a transition graph, so the UI uses a labeled status selector instead of implying Start/Pause/Resume/Fail operations. Completion is disabled or explained when the returned child summary says `can_complete` is false; Core validation remains final. Changing to `Cancelled` requires a clear confirmation.
- Sprint controls follow the exact current `lifecycle.SprintTransition` matrix:

  | Current status | Available actions | Result |
  | --- | --- | --- |
  | `Open` | start, hold, cancel | `Active`, `On Hold`, `Cancelled` |
  | `Active` | hold, complete, cancel | `On Hold`, `Completed`, `Cancelled` |
  | `On Hold` | resume, cancel | `Active`, `Cancelled` |
  | `Completed`, `Cancelled`, `TimedOut` | none | read-only |

  Hold and cancel require a nonblank reason. `TimedOut` is produced by retrieval-attempt policy, not a user action.
- Conflicting work/agent claims, stale state, invalid transitions, missing reasons, and completion blockers are shown as actionable errors. The view refreshes after a conflict so the user sees the current Core state.
- Controls disable during an in-flight request to prevent duplicate submissions. No action runs automatically on selection, startup, or refresh.

The frontend talks only to these existing Core routes: `GET /api/v1/tasks/{taskID}`; `POST /api/v1/tasks/{taskID}/status` with `{"status":"..."}`; and `POST /api/v1/sprints/{sprintID}/{start|hold|resume|complete|cancel}` with a reason object when required. Mutating Go client methods return bounded status-only errors rather than forwarding arbitrary response bodies; frontend error handling must not render arbitrary server response text. The post-conflict refresh supplies current state.

## In scope

- Task selection from the existing Project hierarchy and a focused Task/Sprint detail view.
- Typed Wails/Core read and mutation seams for the existing Task detail, Task status, and Sprint lifecycle API routes.
- Align the Task-detail `can_complete` projection with the Core completion guard, including terminal `TimedOut` Sprints.
- Loading, empty, offline, not-found, conflict, validation, success-refresh, and retry behavior.
- Keyboard-accessible hierarchy selection, focus treatment, labeled status/actions, and automated tests for the critical flows.

## Out of scope

- New Core status values or lifecycle transitions, including Task `Active`/`Failed` and Sprint `TimedOut` controls.
- Direct SQLite access, frontend business rules, a second Core/server, or bypassing existing API/store validation.
- Editing Task/Sprint metadata, creating Tasks/Sprints/Subtasks, assigning agents, or starting background work automatically.
- Activity, agent, file, history, Trail of Thought, metrics, Memory Galaxy, Shadowbox, plugin, and settings tabs.
- Real-time event subscriptions, legacy UI retirement, packaging changes, or deployment.

## Acceptance criteria

1. A user can select a Task from the Project hierarchy, inspect its real Core details and direct children, inspect a direct Sprint, and return to the hierarchy without losing the selected Project.
2. Desktop reads and writes pass through typed Go Wails bindings and the existing Core API; no frontend code accesses SQLite or reimplements Store lifecycle checks.
3. Only Core-supported Task statuses and Sprint lifecycle operations are offered. Required hold/cancel reasons are collected; completion blockers and conflicts remain visible and do not produce false success.
4. The Task-detail `can_complete` value agrees with the Core Task completion guard for every Sprint terminal state, including `TimedOut`.
5. Each successful action is followed by an authoritative refresh of Task detail and Project hierarchy; failures retain the current view and offer a safe retry.
6. Automated frontend and Go tests cover selection, status/action availability, required reasons, blocked completion, duplicate submission prevention, API errors/conflicts, and refresh after success. Existing CLI/API/headless behavior remains unchanged.
7. Svelte checks, frontend tests/build, focused Go tests/vet, and a Windows Wails production build pass. Browser-only fallback behavior is tested separately from desktop bindings; source tests do not claim live-service or GUI runtime proof.

## Implementation constraint

The implementation plan must keep the Task status selector distinct from Sprint lifecycle actions. It must not add unsupported domain behavior to satisfy a generic UI mockup, and the Sprint action matrix above must match `internal/lifecycle.SprintTransition`.
