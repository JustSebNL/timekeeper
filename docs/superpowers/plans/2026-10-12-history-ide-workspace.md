# TimeKeeper File History and IDE Workspace Plan

Goal: Continue the existing Core-owned file history and IDE companion work in the Svelte/Wails UI and VS Code without adding a second database, history service, or workflow authority.

Architecture: Reuse the existing `/api/v1/projects/{id}/file-root`, project files/scan, file metadata/version/compare/restore routes. Extend the typed Wails Core client for desktop; browser development continues through the same HTTP API. Keep the legacy dashboard intact until the new Svelte workspace reaches the relevant parity gate. The VS Code extension remains an explicit-action, numeric-loopback API client; it does not background-scan or upload workspace contents.

## Verified baseline

- `T-11457` is still Open; its history/file-browser source slices SP-11458 through SP-11460 are Completed; SP-11461 is On Hold because the previous installed Core was stale and file history had returned 503.
- The live Core is now reachable at the preferred `http://timekeeper.local` contact. `/api/help` includes file-history routes and `/api/v1/version` reports the running build. `GET /api/v1/projects/P-10053/file-root` returns an empty root; the current file list is empty. Do not claim the TimeKeeper repository has been indexed.
- Legacy `web/file-editor.js` already implements project root selection, scan, editing, immutable versions, branch-aware timeline, compare, and restore. `frontend/src` contains no file-history API or workspace; the VS Code extension currently has Project hierarchy, explicit editor references, and Git-status commands, but no history client/view.
- Preserve the dirty worktree. Do not reset/clean or overwrite unrelated source. Keep the legacy web files and assets as-is unless a narrowly scoped bug is demonstrated.

## Slice A — Typed Wails/Core history API

Files: `internal/desktop/core.go`, `internal/desktop/*_test.go`, `cmd/desktop/workspace.go`, `cmd/desktop/*_test.go`, `frontend/src/lib/api.ts`, `frontend/src/lib/api.test.ts`.

- Add named DTOs and bounded typed methods for project root, tracked files, file metadata, versions, historical content, and compare; add explicit methods for setting the source root, scanning, saving, and restoring.
- Validate IDs/actions and cap responses through the existing Core client policy; expose no Store/database access.
- Pass test-first Core HTTP fixtures for exact paths, payloads, errors, and version-conflict responses before wiring Svelte.
- Preserve Browser HTTP fallback while preferring typed Wails bindings in desktop.

## Slice B — Svelte/Wails File History workspace

Files: `frontend/src/App.svelte`, `frontend/src/lib/navigation.ts` and tests, new `frontend/src/lib/HistoryWorkspace.svelte` and tests, `frontend/src/app.css`, and the typed API from Slice A.

- Add a Files/History workspace sourced only from the selected Project and Core responses.
- Show honest loading, empty, unconfigured, unavailable, and retry states; distinguish an unset source root from an empty indexed file list.
- Make path configuration explicit. Scanning is an explicit, confirmed action; do not scan silently on selection or startup.
- Browse/filter tracked files and chronological/branch-aware immutable versions; preview the selected version as text; compare two versions; offer restore only as an explicit, confirmed action with the expected-current version ID.
- After mutations, refresh the authoritative Core state; keep old versions and branches visible. Use safe text rendering and keyboard-accessible controls.
- No direct SQLite access, local shadow caches, inferred permissions, or silent fallback behavior. Keep the legacy File History panel until parity is verified.

## Slice C — VS Code history companion

Files: `extensions/vscode/lib/client.js`, new history/tree helper modules as needed, `extensions/vscode/extension.js`, `extensions/vscode/package.json`, README and tests.

- Add explicit commands to select a Project/file and inspect recorded versions or open a historical version read-only; use existing Core history endpoints.
- Keep service as the authority; reject non-numeric-loopback origins, do not add background scans/watchers, and do not transmit source text except through an explicitly invoked existing save/snapshot contract.
- Pin exact route and payload contracts, manifest command wiring, safe rendering, and non-mutating default tree selection in tests.

## Slice D — Extended migration conformance and verification

- Keep Wails as a client/adapter over one Core and one SQLite owner. Add no speculative auth, remote failover, message/Calendar service, or independent history database in this vertical.
- Keep `http://timekeeper.local` primary. Document the `127.0.0.1:1618` loopback fallback later in the networking/diagnostics section, not in the opening/local-workspace prose.
- Update the feature matrix/status only for delivered behavior. Do not retire the legacy UI until the document's parity, accessibility, packaging, and rollback gates pass.
- Run focused Go/API/desktop tests, frontend tests/check/build, extension lint/tests, and scoped diff checks. Verify `/api/v1/version`, file-history route availability, installed assets, and browser/IDE behavior separately from source-level tests.
- Update the owning TimeKeeper ToTb checkpoint with verified findings and keep SP-11461 open until its live rollout/history acceptance is proven.
