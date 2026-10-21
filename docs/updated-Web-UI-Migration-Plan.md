# TimeKeeper Desktop and Web UI Migration Plan (Updated)

> This is the actionable replacement for the original [Web UI migration plan](Web-UI-Migration-Plan.md). The original is preserved as historical input. This plan narrows the migration to the architecture that exists and explicitly includes the optional TimeKeeper Store client surface.

## Purpose

Move the human-facing TimeKeeper experience from the legacy browser dashboard toward a Wails + Svelte desktop application without replacing the existing Core, database, HTTP API, CLI, or headless agent workflows.

The desktop UI is another client of TimeKeeper. It is not a second Core, database owner, plugin runtime, or Store service.

## Decisions that govern the migration

1. **One authoritative Core and SQLite owner.** `cmd/server` and `internal/store` remain the authority for durable project state and domain validation. Wails may attach to a compatible Core or supervise an explicitly configured child through the existing desktop lifecycle adapter. It must not open a second Store against the same database, choose another port/database after failure, or stop a Core it did not start.
2. **The existing versioned API is the shared command contract.** The CLI and legacy browser already use it; the Wails adapter forwards typed operations to that Core. Do not add a second application/command layer unless a concrete duplication or missing API capability is demonstrated and tested.
3. **Svelte presents validated state.** It may own view state, filtering, formatting, and interaction. It must not open SQLite, decide workflow legality, duplicate Store validation, or claim state that the backing API cannot prove.
4. **Keep the legacy UI during parity work.** Do not remove or replace its routes/assets until the required feature matrix, supported-platform checks, rollback, and release gates pass.
5. **Keep optional services optional.** Local tracking must remain useful offline and without a Store account. An unavailable/unconfigured Store is not an empty catalogue and must not make Core startup fail.
6. **Feature status and deployment status are separate.** A source implementation, a successful package build, an installed desktop build, and a live Store deployment are different claims. Record evidence at the layer actually tested.

## Current source baseline

The following is a source-level snapshot, not a claim that every desktop binary is installed or running. The TimeKeeper Project task tree is authoritative for changing work status.

| Area | Source-level state | Migration implication |
|---|---|---|
| Core lifecycle | `internal/desktop` and `cmd/desktop` contain the attach/owned-child lifecycle and Wails adapter. The Core remains the only SQLite owner. | Extend this boundary; do not introduce another process or database path. Verify actual Wails runtime behavior separately from Go tests. |
| Wails packaging | `cmd/desktop/wails.json` builds the Svelte frontend into embedded assets. | Keep Wails-specific lifecycle, native capabilities, and packaging in `cmd/desktop`; keep API/domain behavior in Core. |
| Svelte shell | Overview and Projects workspaces, Core health/project reads, persisted workspace selection, and the read-only Project execution hierarchy exist in the current source. | Task detail/actions and other workspaces are separate gates, not implied by the shell. |
| Task/Sprint workspace | Still a separate migration item (`T-11497`). | Read task/sprint data from existing contracts; submit only legal commands through Core and refresh from authoritative state. |
| Activity, ToTb, Memory Galaxy, Metrics, Shadowbox, Plugins/Settings | Separate mapped work (`T-11498`–`T-11502`). | Build only against an endpoint/provider that actually answers; show explicit not-configured states. Memory Galaxy is not the Store. |
| Store client/server | `internal/storeclient` and the CLI support the optional Store protocol, authentication, catalogue/entitlement requests, package verification, and local staging. The separate private `TimeKeeper-Store` service currently uses Node 22+, built-in `node:sqlite` for local development, and the host-provided MariaDB for deployment behind a Cloudflare Worker. | A Svelte Store workspace does not exist yet. Treat it as a client feature, not a Store-backend or database migration. The deployment host's MariaDB is a fixed constraint. |
| Store activation | Staging is not execution. The trusted launcher and connector-specific capability adapters remain gated by the Store security design. | Do not expose an activation/run action until those gates have their own reviewed implementation and tests. |
| Parity and retirement | Feature-matrix, performance/accessibility, and legacy-retirement work are tracked separately (`T-11503`–`T-11505`). | Keep the legacy dashboard until these gates pass; do not equate a visual shell with feature parity. |

`docs/architecture/current-system.md` is a useful audit but includes a snapshot that predates the current Wails manifest and Project hierarchy. Refresh its source snapshot before treating it as a current-state inventory.

## Target architecture

```text
Human
  ├── Legacy browser UI ── HTTP/JSON ──┐
  └── Wails + Svelte ── typed adapter ─┼── TimeKeeper Core API ── Core Store ── SQLite
CLI / Guardian / integrations ─ HTTP ─┘

Wails lifecycle adapter ── attach to existing Core
                         └─ or own one explicitly configured Core child

Future Store workspace: Go client ── HTTPS /v1 ── Cloudflare Worker ── private Store service ── host MariaDB
```

The Wails typed adapter is transport/lifecycle glue. It must not add its own business rules. Browser development may use the existing same-origin API path; Wails uses typed Go bindings backed by the same Core API contract. If a feature needs data the API does not expose, first prove the gap and add the smallest Core endpoint/DTO. Do not reach around Core into SQLite.

The existing persisted event/SSE interface is the first choice for durable activity updates. Use bounded refresh where live delivery is not needed. A Wails event-forwarding adapter may be added if a concrete view needs streaming that the WebView cannot consume; that is transport glue over persisted Core events, not a reason to create a second event bus. Do not add a new in-memory bus pre-emptively.

## Phased delivery and acceptance gates

### Gate 0 — Boundary and feature inventory

Maintain a feature matrix with one row per human-visible capability and columns for: legacy UI, Core API/provider, Svelte UI, CLI/headless behavior, tests/browser evidence, desktop package, and deployment status. Use real route/DTO shapes and current source; distinguish unavailable optional services from empty data.

**Pass when:** every capability in scope has an identified authoritative source and a testable acceptance statement. No broad Go package move or business-rule extraction is part of this gate.

### Gate 1 — Wails/Core lifecycle and desktop shell

Keep the existing attach-or-owned-child lifecycle. Require numeric loopback, compatible health/version, explicit absolute executable/database/UI paths before launching a child, single-instance protection, bounded readiness, and shutdown of only an owned child. Keep the legacy Core API available to `tk`, Guardian, and integrations.

**Pass when:** focused lifecycle tests cover attach, incompatible/occupied Core, missing paths, timeout, early child exit, and owned-vs-attached shutdown; a supported-platform Wails runtime smoke proves attach and owned-child cleanup without creating a second database. A Go unit test alone is not runtime proof.

### Gate 2 — Read-only shell and Project hierarchy

Use the existing health/project/execution-tree contracts to render Overview, Projects, and the nested Category/Task/Subtask/Sprint hierarchy. Keep selection, filtering, and navigation as UI state; refresh from Core after navigation. This gate is implemented in the current source, but future changes must retain its tests and read-only boundary.

**Pass when:** frontend tests, Svelte/TypeScript diagnostics, production build, desktop embed tests, and a browser smoke verify loading, empty/error/retry states, hierarchy ownership/depth, and workspace restoration. Report browser-fixture evidence separately from an installed Wails runtime test.

### Gate 3 — Task and Sprint workspace

Build task detail from the Project execution tree and existing task/subtask/Sprint read contracts. Show only fields returned by Core. Add explicit lifecycle actions through existing validated routes; after each successful command, refresh from Core. Do not introduce frontend status rules, direct database writes, or inferred data.

**Pass when:** route/payload contracts, legal/illegal transition behavior, task/Sprint ownership, refresh-after-write, and browser behavior are tested. Existing API/Store validation remains authoritative.

### Gate 4 — Activity, ToTb, Pulse, and metrics

Map each panel to existing persisted endpoints and document response freshness. Reuse project-scoped ToTb and persisted events. Keep human notes separate from system events, and keep reported values distinct from derived metrics. Only add a Core read projection when the source already has the information but no usable contract exposes it.

**Pass when:** empty, unavailable, and configured states are distinct; every displayed value has a named API field and provenance; SSE/polling has bounded behavior; browser tests prove safe rendering and error states.

### Gate 5 — Store and Extensions workspace

Treat Store as an optional distribution/entitlement client, separate from local project tracking and separate from the private Store server.

The Store UI may surface, through a narrow Go-side adapter backed by the existing `internal/storeclient` contract:

- Store connection/configuration state, including an honest not-configured/offline state;
- account and registered-device status, with secrets redacted;
- catalogue and entitlement results returned by the Store;
- explicit package verification and staging status, including product/version/hash and requested permissions.

Security and ownership rules:

- Svelte must not call Store endpoints directly or receive access/refresh tokens, signing keys, or package bytes as UI state.
- Reuse the existing Go Store client and its credential handling. If desktop sign-in requires a new credential-storage mechanism, design and review that boundary before implementing it; do not put Store secrets in `localStorage`.
- Store-owned decisions—accounts, entitlements, product publication, signing, tickets, and server-side authorization—remain in the private `TimeKeeper-Store` service. Do not reimplement them in Wails or Core.
- Production Store persistence stays on the host-provided MariaDB. PostgreSQL and database migration are out of scope. Local SQLite remains a development convenience; release-critical SQL, transaction, and migration behavior must be verified against the deployed MariaDB version.
- The checked-in Store URL is a reserved example, not a production service address. Show “not configured” rather than fabricating catalogue data or a successful connection.
- “Install” must mean verified and staged, not executed. Keep packages inactive until the separately gated trusted launcher, permission policy, process isolation, and connector adapters are implemented.
- Core tracking and all non-Store workflows continue to work without an account, network, or Store deployment. Payment/checkout, CRM, developer publishing, and Store operator workflows are not part of this desktop UI slice; they require separate Store-owned plans and release gates.

**Pass when:** Store protocol behavior is tested with local HTTP fixtures, auth state is kept out of the WebView, configured/unconfigured/denied states render honestly, package staging cannot activate a plugin, and a live Store smoke is performed only against an explicitly approved deployment. A green fixture test does not prove a production Store exists.

### Gate 6 — Optional workspaces and native capabilities

Add Shadowbox, Memory Galaxy, plugin/settings, native dialogs, notifications, tray/background behavior, and other optional workspaces only through their owning Go/API/provider boundaries. Memory Galaxy remains a cross-project memory subsystem; it is not a Store catalogue. Keep failures isolated so an unavailable optional provider does not take down Core or the ordinary Project workspace.

**Pass when:** each optional panel has a real backing contract, a safe unavailable state, package/platform tests where relevant, and a browser/desktop smoke for its actual user path.

### Gate 7 — Packaging, parity, and legacy retirement

Build reproducible Wails artifacts for each supported platform, verify embedded assets and Core coexistence, and measure startup, memory, idle CPU, interaction latency, and event freshness on named hardware/OS versions. Treat existing performance numbers as hypotheses until measured; define cold/warm startup and foreground/background states before setting release budgets.

Retire the legacy UI only after the agreed feature matrix marks every required capability as parity-verified, CLI/headless behavior remains intact, a rollback artifact and procedure exist, and the release is tested on supported platforms. Optional Store/extension capabilities must be explicitly classified as required, optional, or excluded from parity.

## Explicit non-goals

- Reorganizing `internal/store`, `internal/api`, `internal/cli`, or models into a new domain-package tree solely to match a proposed diagram.
- Creating a central application service, command layer, or event bus before a concrete tested need exists.
- Replacing SQLite, changing stable IDs, or introducing schema changes for presentation-only work.
- Making Wails, Svelte, the Store, or any optional provider a prerequisite for CLI/headless Core operation.
- Direct SQLite or Store HTTP access from Svelte.
- Automatically executing a Store-staged package or shipping purchase/CRM flows before their Store-owned contracts and deployment exist.
- Removing the legacy UI as part of a Wails shell milestone.

## Verification record for every gate

Record the exact source commit/tree state and separate these evidence levels:

1. focused unit/API/Store tests;
2. frontend test, type-check, and production build;
3. Go desktop/embed tests;
4. browser smoke with fixtures;
5. supported-platform Wails runtime/package smoke;
6. Core or Store deployment verification, if that deployment is in scope.

A passing earlier level does not imply a later one. Never report an uninstalled desktop build or unconfigured Store as live.

## Related documents

- [Current-system architecture audit](architecture/current-system.md)
- [Wails single-Core lifecycle design](superpowers/specs/2026-10-10-wails-core-lifecycle-design.md)
- [Wails lifecycle implementation plan](superpowers/plans/2026-10-10-wails-core-lifecycle.md)
- [Store overview](store.md), [Store system architecture](store-system-architecture.md), [Store API](store-api.md), [Store developer guide](store-developer-guide.md), and [plugin security scanning](plugin-security-scanning.md)
- [Documentation index](README.md)
