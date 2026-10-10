# Wails Adapter and Single-Core Lifecycle Design

**Status:** Proposed for review; no implementation authorized by this document alone.

## Goal

Add an isolated Wails v2 desktop adapter that presents the existing Svelte UI while preserving one authoritative TimeKeeper Core process, one SQLite owner, and the existing CLI/API/headless workflows.

## Current evidence

- `cmd/server` acquires `internal/instanceguard` for its file-backed database before opening `internal/store.Store`; it serves the versioned API on loopback and the legacy dashboard separately.
- The CLI and Guardian use Core HTTP endpoints. The migration's first Svelte slice is read-only and calls `/health` and `/api/v1/projects`; its Vite dev proxy keeps browser calls same-origin during development.
- No shared application service exists or is justified for the current UI path. The API remains the command contract; Wails must not open SQLite or duplicate domain rules.
- The server currently has relative defaults (`timekeeper.db`, `.timekeeper/web/index.html`). A desktop-launched Core must receive explicit resolved paths; it must never inherit the desktop process working directory and silently create/use a different database.

## Proposed lifecycle

The Wails app is an API client and optional supervisor for the existing Core executable, not a second Core implementation:

1. On startup, probe the configured loopback Core URL using `/health` and `/api/v1/version`.
2. If a compatible Core is already healthy, attach to it and mark it as externally owned. Wails must not stop it on exit.
3. If no listener/Core is present, launch the packaged or explicitly configured `cmd/server` executable with an explicit absolute database path, UI path, and loopback address. Wait a bounded interval for health and compatible version before presenting the UI.
4. If the address is occupied by an incompatible service, configuration is missing, the database lock is held, or readiness times out, fail visibly. Do not choose another port, create a fallback database, bypass `instanceguard`, or kill an unowned process.
5. On desktop shutdown, stop only a child this Wails instance started. Request graceful termination first; after a bounded wait, force-stop only that owned process. Attached service processes remain untouched.

The resolved database path must come from an explicit desktop configuration or a verified existing Core installation/service configuration. If it cannot be established unambiguously, startup remains attach-only and reports the missing configuration rather than guessing.

## Frontend/API adapter

- Package the built Svelte assets through Wails' `embed.FS` asset path.
- Do not let the WebView call the loopback Core API directly until origin handling is explicitly proven. Use a thin bound Go `CoreClient` that forwards typed operations to the existing HTTP API and returns the same JSON data; it contains transport/lifecycle code only, no Store access or business decisions. Begin with health/version/projects reads; later task scopes add commands deliberately.
- Do not proxy API requests through Wails `AssetsHandler`: current Wails v2 documentation warns of Vite 5+ incompatibility, while this frontend uses Vite 8.3.0. The documented Wails AssetServer also does not support streaming responses on Windows, so the persisted Core SSE stream must not be exposed as a streamed AssetServer response. When live updates are implemented, a Go-side Core SSE client may forward persisted events through Wails runtime events; that is an adapter, not a second event bus.
- Keep the HTTP API usable by `tk`, Guardian, integrations, and the legacy browser while the Wails app runs.

## Failure handling and security

- Treat health, version, process ownership, and database-path resolution as separate facts. A healthy but incompatible server is not permission to spawn another server.
- Log bounded diagnostics without credentials or raw request bodies. Display an actionable offline/version/configuration state in the desktop UI.
- Use loopback only. Never accept a non-loopback Core URL from an untrusted frontend input.
- Preserve the existing instance guard and API/Store validation. No database schema, IDs, CLI commands, or existing UI routes change in this slice.

## Acceptance gates

1. Table-driven Go tests prove attach-vs-launch behavior, version/health rejection, explicit path requirements, startup timeout, and that shutdown only affects an owned child.
2. A Wails build embeds the current Svelte output; the desktop adapter reads real Core health/version/projects through the HTTP contract.
3. With an external Core already running, Wails attaches without opening a second Store and does not stop that service at exit.
4. With no Core running and valid explicit paths, Wails starts one child and `tk doctor`/API reads succeed against it; with missing paths or a held instance lock, it fails without opening a fallback database.
5. Re-run `go test ./internal/cli ./internal/api ./internal/guardian` and focused desktop tests. Source tests are not a substitute for same-machine runtime checks.

## Out of scope

Native tray/background behavior, dialogs/notifications, workspace restore, production installers, supported-platform packaging, and legacy dashboard retirement remain later mapped tasks (`T-11493`–`T-11505`). No refactor of Store or business services is included.

## Reference documentation

- [Wails v2 application development](https://v2.wails.io/docs/guides/application-development/) — lifecycle hooks, bindings, and frontend assets.
- [Wails v2 options / AssetServer](https://v2.wails.io/docs/reference/options/) — embedded assets, handler support, platform request limitations, and lifecycle hooks.
- [Wails v2 manual builds](https://v2.wails.io/docs/guides/manual-builds/) — frontend build and application packaging steps.
