# Current system architecture

**Snapshot:** 2026-10-10 working tree. This is a source-level map for the proposed Wails + Svelte migration. The checkout contains substantial uncommitted changes; verify these paths and behaviors again before making migration decisions from a later revision.

## Review of the migration plan

The plan has sound guardrails: keep Go authoritative, retain the CLI/API/headless workflows, preserve SQLite and existing IDs, introduce the new UI beside the current one, and defer retiring the old UI until parity. Its first implementation step should remain an architecture audit, since this repository already has a broad API and an established server lifecycle.

The sequence needs one decision before Wails scaffolding: **who owns the single Core process and database connection?** `cmd/server` acquires an instance lock for the database before opening it. A Wails process that opens another `store.Store` for the same database would violate that ownership model. Choose and document either (a) Wails as a client of the existing Core service, using the versioned HTTP API, or (b) Wails as the host for the one Core process, with the existing API still served for `tk` and integrations. Do not implement both ownership paths or launch a second Core.

The proposed application service, internal command layer, and event bus are not established abstractions in the current code. The existing Go API already exposes read and write commands, validates inputs, and applies transitions through the Store. The event stream reads persisted project events using polling; it is not an in-memory publish/subscribe bus. Map these boundaries first, then add only the adapter or abstraction that a concrete Wails integration needs. The current dashboard mostly renders API responses and sends API requests, so Phase 2 should identify specific UI-only behavior that must move rather than presume business rules are in JavaScript.

The plan also combines more than twenty major product areas in one migration. Give each phase an independently reviewable acceptance gate and keep the first milestone read-only. Treat tray behavior, crash recovery, packaging, metrics parity, and performance as separate milestones after the shell and read-only data path work. The stated performance numbers are targets, not acceptance evidence, until measured on supported platforms.

The Wails v2 choice is reasonable for a stable first desktop integration: official Wails documentation currently identifies v2 as the stable release, while v3 remains beta. Keep the dependency isolated as the plan proposes and recheck release status when implementation reaches framework setup. [Wails v2 documentation](https://v2.wails.io/docs/gettingstarted/installation/) · [Wails v3 status](https://v3.wails.io/status/)

## Process and interface boundaries

```text
tk CLI ──HTTP/JSON──┐
                    ├── cmd/server ── internal/api ── internal/store ── SQLite
Browser dashboard ──┘                       │
                                            ├── optional history / knowledge / observer / recovery services
                                            └── persisted project events ── SSE polling
```

- `cmd/tk/main.go` resolves the service URL and delegates CLI commands to `internal/cli`. Core API commands use the CLI's HTTP client against the versioned service API; the separate `internal/storeclient` package targets the optional plugin Store.
- `cmd/server/main.go` is the Core process entry point. It validates configuration, acquires `internal/instanceguard` for the target database, reserves the canonical listener, opens `internal/store.Store`, and mounts `internal/api` plus the static dashboard. It also owns optional runtime wiring.
- `internal/api` registers the versioned HTTP/JSON contract. Handler constructors receive `*store.Store`; handlers call Store methods for projects, tasks, sprints, agents, notes/messages, usage, imports/exports, and related records. Optional capabilities are injected through interfaces or explicit runtime configuration.
- `internal/store` owns the authoritative SQLite connection, schema migrations, persistence, and much of the domain validation/transition behavior. `store.Open` enables WAL, a five-second busy timeout, synchronous NORMAL, cache sizing, and foreign keys before migration.
- Supporting packages isolate selected capabilities: `internal/lifecycle` handles lifecycle rules; `internal/planning` handles planning drafts; `internal/history` implements tracked-file history; `internal/knowledge` wraps an optional provider; `internal/guardian` owns pulse/recovery behavior; `internal/plugin` builds optional providers; `internal/observer` is an optional event journal boundary.

## UI and frontend dependencies

- `cmd/server` serves `web/` as static files from the configured dashboard path. The current UI uses `web/index.html`, `web/timekeeper.css`, `web/timekeeper.js`, and `web/file-editor.js`.
- The dashboard uses same-origin `/api/v1/...` requests. It renders project/task and dashboard state, manages browser view state and user interaction, and submits commands to the API. The server and Store remain authoritative for durable state and business transitions.
- The page loads a locally bundled Bootstrap stylesheet and CodeMirror bundle. `web/editor/` is a small npm package for the CodeMirror adapter; it is not the application frontend build. `frontend/` now contains the first isolated Svelte/Vite read-only overview, API client, and tests. There is not yet a Wails project manifest or desktop adapter.
- Database access is absent from the browser layer. A Wails UI should preserve that rule and use an explicitly chosen adapter to the one Core instance.

## Confirmed browser/Core behavior boundary

- Existing dashboard actions submit commands to versioned API routes. Examples include project/task/category status changes and Sprint transitions. The API decodes request inputs and delegates durable operations to Store methods; Store validates and applies the state transitions. Browser prompts and required-field checks improve interaction but are not the authority for persisted state.
- `durationToSeconds` parses the dashboard's human-readable estimate field into seconds. Store validates the resulting numeric estimate; the parser is form-input handling, not a second domain transition rule.
- The overview derives a few display aggregates in JavaScript (status counts and cross-project usage totals). Each project's usage endpoint already returns authoritative token totals and per-session costs priced by the server-owned catalogue. The current browser combines those project projections for one overview card, including unpriced/mixed-currency handling. This is a presentation aggregation to map into the later metrics workspace (`T-11500`), not justification for a new shared command/service layer or API-wide aggregation endpoint now.
- Audit conclusion for `T-11487`: no duplicated project/task/Sprint write rule was found that warrants extraction. New Svelte surfaces should call the existing API for reads and commands, keep form formatting local, and avoid reproducing Store validation. Revisit only if a concrete API limitation or parity requirement is demonstrated.
- Boundary decision for `T-11488`: the CLI calls the versioned HTTP contract through its own HTTP client in `internal/cli`; API handlers invoke the authoritative Store. Wails can use the same Core API contract, so a second Go application/command façade would add indirection without consolidating a duplicated rule. Keep the API as the shared UI command boundary for now; reconsider a Go service only if desktop hosting needs an in-process capability that cannot safely be expressed through the API. (`internal/storeclient` serves the optional plugin Store, not the Core API.)

## State, events, workers, and integrations

- Durable state is in SQLite, opened by the Core server. The Store serializes record rules and applies schema migrations. Backup mode uses `Store.BackupTo` and exits without starting the service.
- The server starts the optional Pulse Guardian goroutine and an optional low-frequency HTTP health keep-alive. It can configure a local recovery handler, tracked-file history, a knowledge provider, an event observer, and the plugin manifest root from explicit flags/environment values. Optional capabilities degrade or remain unavailable when not configured.
- `GET /api/v1/events/stream` and the project-scoped endpoint use Server-Sent Events. `internal/api/events_stream.go` reads persisted events through `Store.ListEventsAfterID`, resumes by event ID, starts at 250 ms polling and backs off to 1 s while idle, and emits a 15 s heartbeat. This is sufficient for initial UI refresh/activity updates; it has polling/database-read cost but no new event infrastructure. For `T-11489`, use one shared frontend `EventSource` and fan out relevant events locally when live views are added; add an in-memory bus only if a concrete non-persisted consumer appears.
- Agent workflows use the CLI, API, Guardian, and runtime adapters. They do not depend on browser automation. Keep these interfaces operational when adding the desktop shell.
- Compatibility baseline for `T-11490`: `cmd/server` mounts `/api/` and `/health` directly to the API handler, independently of its dashboard static-file fallback. Core CLI commands issue HTTP requests through `internal/cli`; `cmd/guardian` and `internal/guardian` call API endpoints directly and do not load frontend assets. Focused source tests passed on this snapshot: `go test ./internal/cli ./internal/api ./internal/guardian`. This is not desktop coexistence proof: repeat CLI/API/Guardian checks with Wails running after `T-11491`/`T-11492`.
- Plugins integrate through Go-side registries and optional service interfaces. Wails is not the plugin execution boundary.

## Migration implications

1. Resolve Core ownership and desktop/server coexistence before adding a Wails entry point. Preserve the single-instance database guard and canonical API behavior.
2. Reuse the existing API for a read-only first UI milestone unless a measured limitation requires direct Go bindings. If bindings are needed, have them call the same Core services/Store instance used by the API in the selected process model.
3. Do not create parallel business rules. For every proposed `GetProjects`, task action, or event subscription, map it to the existing API route and Store method first.
4. Reuse the persisted event stream for initial live updates. Introduce an in-memory bus only when a concrete consumer requires events that the durable journal cannot provide.
5. Keep the legacy dashboard and CLI/API in service during parity work. Add a feature matrix and phase acceptance evidence before changing packaging or defaults.
6. Keep Wails-specific lifecycle, dialogs, notifications, and packaging at the desktop adapter boundary. The server, CLI, and headless integrations must remain usable without Wails.

## Primary source locations

- Server lifecycle and static UI: `cmd/server/main.go`
- CLI entry and Core API client: `cmd/tk/main.go`, `internal/cli/` (the optional plugin Store client is `internal/storeclient/`)
- HTTP contract and handler wiring: `internal/api/server.go` and sibling files under `internal/api/`
- SQLite ownership and migrations: `internal/store/store.go`
- Persisted event stream: `internal/api/events_stream.go`
- Current dashboard: `web/index.html`, `web/timekeeper.js`, `web/timekeeper.css`, `web/file-editor.js`
- Optional tracked-file service: `internal/history/`
- Runtime guard: `internal/instanceguard/`
