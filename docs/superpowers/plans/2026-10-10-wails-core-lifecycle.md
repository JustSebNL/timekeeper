# Wails Adapter and Single-Core Lifecycle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Wails v2 desktop shell that safely attaches to an existing TimeKeeper Core or supervises one explicitly configured child without creating a second database owner.

**Architecture:** A new `internal/desktop` package owns Core probing, typed HTTP reads, path validation, child launch, readiness timeout, and shutdown ownership. `cmd/desktop` is a thin Wails adapter over that package. The server gains an opt-in parent-stdin-close shutdown path so only a Wails-owned child can be asked to stop gracefully; ordinary server and CLI behavior remain unchanged.

**Tech Stack:** Go 1.26.1, Wails v2.15.0, `net/http`, `os/exec`, `embed.FS`, existing `/health`, `/api/v1/version`, and `/api/v1/projects` contracts.

**Spec:** [Wails adapter and single-Core lifecycle design](../specs/2026-10-10-wails-core-lifecycle-design.md)

## Global Constraints

- Use Wails v2; do not add a second Core implementation or direct SQLite access from the desktop adapter.
- Use only numeric loopback Core URLs; reject remote hosts and non-HTTP schemes.
- Attach to a healthy compatible Core without opening a Store or stopping that process.
- Start a child only when executable, absolute database path, and absolute dashboard path are explicit and validated.
- Never select another port, create a fallback database, bypass `internal/instanceguard`, or terminate a process the desktop app did not start.
- Child shutdown is opt-in via a private stdin pipe; normal server/service startup behavior stays unchanged.
- Do not proxy API calls through Wails `AssetsHandler`; keep the Core HTTP API available to `tk`, Guardian, integrations, and the legacy browser.
- T-11492 owns embedding the Svelte build and connecting its real read-only project view; this plan supplies the desktop Core client and Wails lifecycle seam only.
- Preserve unrelated dirty worktree changes; no commit, deployment, installer, or legacy UI retirement in this slice.

## Review Focus

1. A compatible Core already owns the configured address: attach without starting a child and leave it alive on desktop shutdown.
2. An occupied address serves unhealthy, malformed, or incompatible responses: fail visibly and never launch a second Core.
3. No Core is listening but launch configuration is missing, relative, or points to absent dashboard assets: fail before process start or database creation.
4. A launched child fails readiness or exits early: report the cause, close only its stdin, and reap only that child.
5. Parent stdin closes while a child is serving: shut down both Core listeners gracefully; a normal service with the option disabled must not react to stdin EOF.

---

### Task 1: Add the Core client and ownership-aware desktop lifecycle

**Files:**
- Create: `internal/desktop/core.go`
- Create: `internal/desktop/core_test.go`
- Create: `internal/desktop/process.go`
- Create: `internal/desktop/process_test.go`

**Interfaces:**
- `Config` has `CoreURL`, `CoreExecutable`, `DatabasePath`, `DashboardPath`, `StartupTimeout`, and `ShutdownTimeout` fields.
- `CoreClient` exposes `Health(ctx) (Health, error)`, `Version(ctx) (Version, error)`, and `Projects(ctx) ([]model.Project, error)` using only HTTP GETs to the existing API.
- Lifecycle errors include `ErrNotConfigured`, `ErrIncompatibleCore`, and `ErrStartupTimeout`, so the Wails shell can show a bounded actionable status without parsing error strings.
- `Session` exposes `Client() *CoreClient`, `Owned() bool`, and `Close(ctx) error`.
- `StartOrAttach(ctx, cfg, launcher) (*Session, error)` probes first; it returns an unowned session for a compatible server, otherwise launches only when all explicit paths are absolute and valid.
- `Launcher.Start(ctx, executable string, args []string) (ManagedProcess, error)` and `ManagedProcess` methods `Wait() error`, `CloseInput() error`, and `Kill() error` isolate OS process control for deterministic tests.

- [x] **Step 1: Write failing table-driven probe and configuration tests**

Add `TestStartOrAttach` cases for: healthy `/health` plus matching `/api/v1/version` attaches with `Owned()==false`; non-200 health, malformed JSON, wrong service, and incompatible version return errors without invoking the fake launcher; no listener launches only with all three explicit absolute paths; relative or missing DB/dashboard paths return errors without invoking the launcher. Use `httptest.Server`, a fake launcher counting starts, and a temporary directory containing `index.html`, `timekeeper.css`, and `timekeeper.js` for the valid child configuration.

- [x] **Step 2: Run the lifecycle tests and confirm the expected red**

Run: `go test ./internal/desktop -run 'TestStartOrAttach' -count=1`
Expected: FAIL because package and lifecycle functions do not exist yet.

- [x] **Step 3: Implement bounded Core probing and explicit configuration validation**

In `core.go`, define JSON response types for `/health` and `/api/v1/version`; accept only HTTP URLs with a numeric loopback IP and non-empty port. Consider the Core compatible only when health is successful with `status == "ok"`, JSON identifies `service == "TimeKeeper"`, and the server version equals the desktop build's `version.Get().Version`. Use an `http.Client` with request contexts and a 2-second per-request timeout. Validate dashboard assets using the same three regular/readable-file requirements as `cmd/server.validateServerConfig`; require an absolute DB path with an existing parent directory and an absolute existing regular Core executable before launch. Never retry a rejected occupied endpoint by launching.

- [x] **Step 4: Implement the typed read-only CoreClient**

Implement `Health`, `Version`, and `Projects` as bounded GETs. Decode the current API response shapes, reject non-2xx responses with status-only errors, and do not expose response bodies in logs/errors. Keep each method free of Store or domain logic.

- [x] **Step 5: Write failing child lifecycle tests**

Add `TestLaunchUsesExplicitPathsAndLoopback`, `TestStartupTimeoutStopsOnlyOwnedChild`, `TestChildExitBeforeReadyFails`, and `TestCloseDoesNotStopAttachedCore`. The fake launcher records arguments and returns a fake process; the fake process exposes a controllable `Wait` result and records stdin-close/kill calls. Use an explicitly configured free loopback port in launch tests so no test competes with the live canonical service; assert child `-addr` exactly matches that configured URL (the desktop default remains `127.0.0.1:1618`), along with `-db <absolute DB>`, `-ui <absolute dashboard>`, and `-shutdown-on-stdin-close`. Timeout must close input and then kill only that returned child, while an attached session performs no process operation.

- [x] **Step 6: Run the child tests and confirm the expected red**

Run: `go test ./internal/desktop -run 'TestLaunchUsesExplicitPathsAndLoopback|TestStartupTimeoutStopsOnlyOwnedChild|TestChildExitBeforeReadyFails|TestCloseDoesNotStopAttachedCore' -count=1`
Expected: FAIL until launch, bounded readiness polling, and session ownership are implemented.

- [x] **Step 7: Implement process launch and session close semantics**

Implement the OS launcher with `exec.CommandContext`, a dedicated stdin pipe, and a goroutine that records `Wait()`. `StartOrAttach` derives the child `-addr` from the already-validated `CoreURL` host and port, preserving the default `127.0.0.1:1618` and never selecting a fallback port; it polls only that URL until ready, child exit, context cancellation, or `StartupTimeout`. `Session.Close` is a no-op for attached sessions; for owned sessions it closes stdin, waits at most `ShutdownTimeout`, and calls `Kill` only on timeout. Return the declared error categories without leaking HTTP bodies.

- [x] **Step 8: Run desktop package tests and inspect the focused diff**

Run: `go test ./internal/desktop -count=1`
Expected: PASS for probe, configuration, typed reads, launch arguments, timeout, and ownership cases. Run `git diff --check -- internal/desktop` and inspect only these new files.

### Task 2: Add graceful shutdown for an explicitly supervised server child

**Files:**
- Modify: `cmd/server/main.go`
- Modify: `cmd/server/main_test.go`

**Interfaces:**
- Add server flag `-shutdown-on-stdin-close` (default `false`).
- Add `watchParentInput(ctx context.Context, input io.Reader, enabled bool, stop context.CancelFunc)`; it starts a reader only when enabled, calls `stop` only on EOF while the context is still live, and exits quietly on context cancellation.
- Add `serveUntilShutdown(ctx context.Context, primary *http.Server, primaryListener net.Listener, proxy *http.Server, proxyListener net.Listener) error`; it waits on context cancellation or a Serve error, then shuts down both servers with a 5-second context before returning to deferred Store/lock cleanup.
- Main creates the signal context, starts `watchParentInput` only when the flag is true, and passes the already-bound primary/proxy listeners to `serveUntilShutdown`.

- [x] **Step 1: Write failing tests for opt-in parent-close and graceful listener shutdown**

Add `TestWatchParentInputStopsOnEOF` using `io.Pipe`, asserting the cancel function is not called before the writer closes and is called after EOF. Add `TestWatchParentInputDisabledIgnoresEOF`, close the writer with `enabled=false`, and assert the context remains active. Add `TestServeShutdownClosesPrimaryAndProxy` using two `httptest` listeners and a cancellable context; verify both servers stop accepting new connections and the helper returns cleanly after `http.ErrServerClosed`.

- [x] **Step 2: Run the focused server tests and confirm the expected red**

Run: `go test ./cmd/server -run 'TestWatchParentInputStopsOnEOF|TestServeShutdownClosesPrimaryAndProxy' -count=1`
Expected: FAIL because the opt-in watcher and graceful serving path do not exist.

- [x] **Step 3: Implement opt-in stdin EOF and signal-aware graceful shutdown**

Add the default-off flag without changing other server defaults. Replace the blocking-only `Serve` tail with `serveUntilShutdown`; its testable context is cancelled by `signal.NotifyContext` for `os.Interrupt` and, only when enabled, by `watchParentInput` on stdin EOF. On shutdown call `Shutdown` for the primary and any started proxy server using one 5-second context. Preserve the existing acquired listener, `instanceguard` lock, database, guardian, and logging defer order. Do not add a public HTTP shutdown endpoint.

- [x] **Step 4: Run server regression tests and compatibility checks**

Run: `go test ./cmd/server -count=1`
Expected: PASS, including existing configuration and listener tests plus the new EOF/shutdown cases. Run `go test ./internal/cli ./internal/api ./internal/guardian -count=1` to verify the command/API/headless boundary remains unchanged.

### Task 3: Add the isolated Wails v2 desktop entrypoint

**Files:**
- Create: `cmd/desktop/main.go`
- Create: `cmd/desktop/app.go`
- Create: `cmd/desktop/assets/index.html`
- Create: `cmd/desktop/app_test.go`
- Create: `cmd/desktop/wails.json`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- `DesktopApp` owns `*desktop.Session` and a bounded startup status; Wails hooks call `Startup(context.Context)` and `Shutdown(context.Context)`.
- Bind context-free `DesktopApp` methods `Status() DesktopStatus`, `Health() (desktop.Health, error)`, `Version() (desktop.Version, error)`, and `Projects() ([]model.Project, error)` to Wails; each wrapper creates a bounded request context for the internal `CoreClient`. Later Svelte bindings consume these methods in T-11492.
- Wails options use `assetserver.Options{Assets: embeddedAssets}`, bind only the desktop adapter/client, and do not start another HTTP listener.

- [x] **Step 1: Add failing app configuration and shutdown tests**

Add `TestStartupReportsUnconfiguredCore` asserting an unreachable Core plus absent launch paths produces a visible not-configured status without process launch. Add `TestShutdownClosesOwnedSession` and `TestShutdownLeavesAttachedSessionRunning`, using the fake launcher/session from `internal/desktop` through an injected lifecycle factory.

- [x] **Step 2: Run the desktop tests and confirm the expected red**

Run: `go test ./cmd/desktop -run 'TestStartupReportsUnconfiguredCore|TestShutdownClosesOwnedSession|TestShutdownLeavesAttachedSessionRunning' -count=1`
Expected: FAIL because the Wails adapter does not exist.

- [x] **Step 3: Add Wails v2.15.0 and the thin adapter**

Add the pinned Wails v2.15.0 module dependency. Build `cmd/desktop` with explicit flags `-core-url`, `-core-exe`, `-db`, and `-ui`; default only `-core-url` to `http://127.0.0.1:1618`. If attach succeeds, show connected status. If attach fails and launch configuration is incomplete, keep the window open with an actionable configuration/offline status. If launch is configured, call `StartOrAttach`; never panic or silently fall back to another DB/port. Implement `OnShutdown` to close the session with a bounded context.

- [x] **Step 4: Embed a minimal diagnostic shell and wire Wails lifecycle hooks**

Embed `cmd/desktop/assets/index.html` with `embed.FS`, configure the Wails v2 AssetServer, and bind `DesktopApp` plus the typed `CoreClient`. The shell displays connected, offline, incompatible-version, and missing-configuration states as plain text; it is temporary and contains no duplicate project UI or API logic. Keep the window's API path inside Go bindings, not direct browser fetches to loopback.

- [x] **Step 5: Run focused tests, build, and final source checks**

Run: `go test ./internal/desktop ./cmd/desktop -count=1`; `go test ./cmd/server ./internal/cli ./internal/api ./internal/guardian -count=1`; `go build ./cmd/desktop`; and `git diff --check`.
Expected: all tests/build pass; the desktop entrypoint compiles and owns no HTTP listener or Store. Record that embedding the Svelte output and rendering real project data remain T-11492, not a claim of full migration completion.

---

## Spec Coverage and Follow-on Boundary

- Spec lifecycle rules 1, 2, 3, 4, 5 and failure/security rules are covered by Tasks 1–3 and the review-focus tests.
- Spec acceptance gate 1 is covered by `internal/desktop` tests.
- Spec acceptance gates 3 and 4 are covered by lifecycle tests plus a manual same-machine smoke test in T-11491 after user approval: attach to the existing Core, launch with a temporary isolated DB and valid dashboard assets, verify `/health` and `/api/v1/projects`, close the desktop, and verify only its child exits.
- Spec acceptance gate 2's Svelte embedding and real project presentation is owned by the next mapped Task T-11492; this plan deliberately provides the typed Go read path and temporary Wails asset seam first.
- Final verification also reruns `go test ./internal/cli ./internal/api ./internal/guardian -count=1`; source tests do not prove the installed service or production desktop package.
