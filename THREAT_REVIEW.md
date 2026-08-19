# Time Keeper Threat Review

Review date: 2026-08-10

## Scope

This review covers the local Go HTTP server, SQLite authority, read-only Sprint Pulse attention endpoint, default-on Pulse Guardian delivery loop, dashboard, `tk` CLI, VS Code companion, portable export, SQLite backup mode, and installer posture in the current source tree.

## Security boundary

Time Keeper is a local single-user/process-boundary tool, not a multi-tenant service and not an authenticated network API.

The server only accepts explicit numeric loopback listeners (`127.0.0.1` or `::1`). Wildcard, hostname, and remote bind addresses fail at configuration validation. There is deliberately no option to bypass this in the current binary.

Consequences:

- A process that can act as the local user can read or mutate the local service. This is outside Time Keeper's security boundary.
- `X-Agent-ID` is provenance metadata, not authentication. Any local client can supply it; do not use it for authorization or non-repudiation.
- Remote access requires a separate future design with authenticated transport, authorization, rate limits, and an explicit deployment boundary. Do not place the current service behind a public proxy.

## Assets and protections

| Asset | Current protection | Remaining responsibility |
|---|---|---|
| SQLite state | parameterized SQL, foreign keys, WAL, SQLite snapshot backup | protect the database directory with OS permissions and backups at rest |
| Sprint Pulse attention | a `GET`-only calculation from durable Sprint history; no notification rows, background worker, outbound HTTP, or delivery credentials | callers decide whether and when to poll; Pulse data remains inside the existing loopback-only local-process boundary |
| Pulse Guardian | default-on periodic lease evaluation (5m; override via `-pulse-guardian-interval`, disable with empty interval); durable Pending/Acknowledged nudge and delivery evidence; callback URL restricted to plain HTTP numeric loopback with an explicit port; 3-second timeout; redirects refused; a 2xx response plus `X-Timekeeper-Pulse-Accepted: v1` is required; confirmed callbacks are not repeatedly delivered before acknowledgement | same-user processes can still register/update a Guardian; callback acceptance proves only receipt, not recovery; registered local Guardian must own any interrupt/restart action and acknowledge only after recovery |
| Browser-rendered Project data | explicit DOM construction with `textContent`; dashboard contract forbids `innerHTML`; HTML, CSS, and JS are static same-origin assets; CSP constrains scripts/styles/connections to same origin and disables object/base/frame embedding | keep untrusted data out of markup/attributes and preserve the explicit asset allowlist |
| Mutation endpoints require `application/json`, cap bodies at 1 MiB, and reject unknown fields | local clients remain trusted by the local-only boundary |
| HTTP exposure | loopback-only configuration, no CORS opt-in, `nosniff`, frame denial, no-referrer, no-store headers | keep default binding local |
| VS Code companion | uses only the public HTTP API; validates a configured numeric-loopback HTTP origin; no SQLite access, shell execution, arbitrary SQL, workspace file reads, server startup, or persistent task cache | a same-user local process remains within the existing local-process trust boundary; do not add remote endpoints or automatic code-context submission without a distinct security review |
| Backups | SQLite `VACUUM INTO` snapshot; destination must be new and parent must exist; owner-only mode requested after creation | backup files are unencrypted; on Windows place them in a user-private ACL-protected directory |
| Project export | explicit project-scoped JSON endpoint/CLI output | exports may contain sensitive notes and history; store them accordingly |
| Local planning pipeline | only configured plain-HTTP numeric-loopback endpoints; bounded request/response sizes, 60s timeout, and redirects refused before any follow-up request | any local-user process can still receive Project context through an intentionally configured local endpoint |
| Model-proposed hierarchy | strict versioned JSON validation, immutable Review artifact, separate explicit one-time apply transaction | models may still propose poor work; human review remains required |

## Findings and decisions

### Closed: accidental network exposure

Server startup now rejects wildcard and remote interfaces before database startup. This prevents the most likely configuration mistake from converting an unauthenticated local API into a LAN service.

### Closed: unsafe installer surface

`install.sh` is a deliberately narrow local-source bootstrap, not a release installer. It accepts only the clean checkout it is run from, whose official `origin` and locally verified `origin/main` exactly match `HEAD`. It archives that pinned commit and stages the result under the checkout's ignored `.timekeeper/app` root. A later run refreshes the app files there while preserving its existing runtime state. It neither downloads code, elevates privileges, modifies `PATH`, manages a service, nor starts a server. Disposable harness tests prove the clean-source, repository-contained, state-preserving refresh, and no-start boundary.

### Closed: incomplete SQLite file copying

Backup mode uses SQLite's `VACUUM INTO` rather than copying a main database file without WAL state. It refuses overwrite and verifies independent reopening in automated tests.

### Mitigated: browser form mutation attempts

Mutation endpoints require `application/json`. Dashboard and `tk` use that type; ordinary cross-site HTML form requests use form content types and are rejected. The service also emits no CORS permission headers. This is a mitigation, not an authentication boundary.

### Mitigated: Pulse Guardian callback escalation

A stuck agent cannot reliably poll its own attention state, so Guardian lease evaluation runs independently of the watched agent's work loop. The callback is constrained to a user-configured, plain HTTP numeric-loopback URL with an explicit port; names, remote addresses, userinfo, query strings, fragments, redirects, and non-2xx responses are rejected. The callback must explicitly return `X-Timekeeper-Pulse-Accepted: v1`; receipt is still not recovery. Nudge state, delivery attempts, first confirmed delivery, acknowledgement, and owner are durable SQLite evidence. Failed deliveries retry on later configured Guardian ticks, while accepted-but-unacknowledged nudges are not spammed.

Time Keeper deliberately does not execute arbitrary commands or control processes. A separately owned local Guardian adapter makes any interrupt/restart/replacement decision. This avoids a data-plane callback becoming a generic code-execution surface, but it means an installed Time Keeper alone cannot know how a particular agent runtime should be interrupted.

### Closed: VS Code companion broad-access risk

The initial companion is a constrained local API client. It validates only a plain-HTTP numeric-loopback origin, launches no process, reads no workspace file, stores no task cache, and has no code-context or cloud-submission command. Sprint mutations are exposed only through explicit context-menu commands; clicking a Sprint item never changes durable state.

### Accepted residual: no authentication or encryption

There is no user authentication, authorization, TLS, database encryption, backup encryption, or secure-secret store. This is acceptable only while Time Keeper is local-only and its data directory is protected by the operating system. It is a release blocker for any remote/shared deployment.

### Accepted residual: local process trust

Any process running with the user's local authority may interact with the loopback listener and may read files the user can read. This tool cannot defend against an already-compromised local user account.

## Release gate

The source is ready for local use after ordinary build/test verification and the narrow user-owned local bootstrap. It is **not** approved for public network deployment, unattended privileged installation, multi-user access, or a downloadable release installer.

Before providing downloadable or signed install artifacts, require all of:

1. reproducible, signed release artifacts with checksums;
2. platform-specific service model and rollback tests;
3. explicit supported-platform matrix;
4. installation test coverage in disposable environments;
5. a deliberate decision on local-only versus authenticated remote mode;
6. backup-retention, restoration, and encryption policy.
