# Guardian Control-Plane and Port Consolidation

Status: Phases 1–3 for the artifact-only receiver are implemented in source. The
canonical `POST /api/v1/guardian/recover` route shares the legacy receiver's
validation, artifact, and acknowledgement path when `-guardian-state-dir` is
configured, and the duplicate receiver is now opt-in rather than started by the
default launchers. Existing 1619 receivers and registrations remain supported
during migration; no live listener has been removed. P-10053/T-10473 tracks the
remaining deployment migration and retirement checks.

## Decision

The friendly contact point is `http://timekeeper.local/`; the canonical process listener remains numeric loopback at `timekeeper.local:1618`. `GET /api/v1/connection-info` advertises the friendly dashboard/API hosts and the actual port suffix when the proxy uses a non-default port.

Pulse Guardian already runs inside the TimeKeeper server process. The separate
`127.0.0.1:1619` receiver duplicates a small part of the control plane and
should be retired from the default architecture.

Agent runtimes may have their own ports. Those are not TimeKeeper ports:

```text
127.0.0.1:8642  Hermes gateway (external runtime integration)
127.0.0.1:2222  SSH/service infrastructure (unrelated)
```

TimeKeeper must not bind or proxy an agent runtime's port merely to reduce the
number shown by `ss`. The boundary is an authenticated adapter contract, not a
port collision.

## Target topology

```text
                    one canonical listener
                    timekeeper.local:1618
                           |
       +-------------------+-------------------+
       |                   |                   |
   Core API             Pulse Guardian       Guardian recovery
   /api/v1/*            in-process tick       /api/v1/guardian/recover
       |                   |                   |
       +-------------------+-------------------+
                           |
              agent-side AGENT_WAKE_COMMAND
                           |
       +-------------------+-------------------+
       |                   |                   |
 local resume       supervisor IPC       runtime gateway
                                              |
                                  Hermes 8642 / other runtime
```

The recovery route belongs to the TimeKeeper API because it validates the
TimeKeeper pulse signal, records recovery evidence, and acknowledges a nudge.
The agent-side adapter remains outside Core because only the agent runtime knows
how to cross its own sandbox or supervisor boundary.

## Windows service boundary

The Windows Core service uses Bend2Will's `obey` service manager with `LocalService`. The installed service has not been migrated in this workspace; the migration and health check must run before describing Bend2Will as deployed.

```text
TimeKeeper Core service (Bend2Will; delayed start, LocalService)
  -> timekeeper.exe
  -> API + dashboard + in-process Pulse Guardian
  -> timekeeper.local:1618
```

The agent-side wake consumer is a different component:

```text
tk guardian wake --agent <stable-agent-id> --adapter <trusted-executable> --poll
  -> agent-side adapter
  -> agent runtime supervisor/gateway
```

Do not run the agent-side consumer inside the Core service by default. A
Core service running as `LocalService` does not own the interactive agent's
Hermes profile, WSL user environment, runtime session, or credentials. Putting
it there would make the installation appear durable while silently breaking the
adapter boundary.

The portable installation contract is therefore:

- Linux uses systemd. The Windows installer uses Bend2Will `obey` with `LocalService`, delayed startup, SCM recovery, and bounded logs.
- The agent runtime supervises its own adapter where possible.
- If an agent needs a Windows-side watcher, install a separate per-agent
  supervisor under the agent's user context, not as the Core `LocalService`
  account.
  runtime profile are explicitly configured.
- A user-context Task Scheduler logon trigger is an acceptable Windows
  alternative when a service account cannot access the runtime profile.
- The first-use harness must report the Core service and agent watcher as two
  distinct health checks; one must not be presented as proof of the other.

This keeps one TimeKeeper port without confusing OS process supervision with
agent-work attention recovery.

## Why not merge every port

A port belongs to the process that owns its protocol and lifecycle. Hermes's
8642 gateway has its own authentication, scheduler, sessions, and restart
semantics. Making TimeKeeper bind 8642 would couple Core to Hermes and would
break Claude/Codex/OpenCode/custom-runtime integrations.

The correct reduction is:

- eliminate the dedicated TimeKeeper 1619 listener;
- keep one TimeKeeper 1618 listener;
- let adapters make authenticated outbound calls to runtime gateways;
- use Unix sockets, named pipes, or loopback ports for runtime-specific
  transports when available;
- keep the public/friendly proxy separate from the canonical API listener.

## Current duplication

The current `cmd/guardian` / `internal/guardian/receiver.go` process:

1. listens on `127.0.0.1:1619`;
2. parses `timekeeper-pulse-guardian/v1` recovery signals;
3. writes a local recovery artifact;
4. POSTs nudge acknowledgement back to `timekeeper.local:1618`;
5. returns `X-Timekeeper-Pulse-Accepted: v1`.

The core server already owns the database and exposes Guardian status, Pulse,
agent progress, nudge history, and acknowledgement routes. Maintaining a
second listener creates extra startup, registration, failure, and port-conflict
states without creating a useful security boundary: both endpoints are local
loopback surfaces and the receiver still calls Core for acknowledgement.

## Migration phases

### Phase 1 — additive canonical route

Implemented in source. `internal/guardian` now exposes a reusable recovery
handler with explicit dependencies:

- existing private state directory/artifact writer;
- caller-supplied nudge acknowledgement function;
- bounded JSON body, strict `application/json` content type, and schema checks;
- acceptance header/JSON response.

The canonical API mounts the handler as:

```text
POST /api/v1/guardian/recover
```

The route is enabled only when the server receives an existing private
`-guardian-state-dir` (or `TIMEKEEPER_GUARDIAN_STATE_DIR`). Without that explicit
configuration it returns `503 guardian_recovery_unavailable` instead of guessing
a writable location. The existing `/v1/recover` receiver route remains as a
compatibility wrapper, and tests cover the shared validation and
acknowledgement path.

### Phase 2 — canonical registration

Implemented in source for the artifact-only receiver. `cmd/guardian -register`
reads `guardian/status` and advertises the canonical recovery callback when the
server exposes `guardian_recovery_url`:

```text
http://guardian.local:1618/api/v1/guardian/recover
```

Older Cores, unavailable status responses, or missing fields fall back to the
existing `http://127.0.0.1:1619/v1/recover` registration, keeping compatibility
retryable during migration.

`tk init` intentionally continues to register the self-nudge receiver's `/pulse`
URL. That callback admits a real work cycle; the artifact-only canonical route
is not a substitute for worker admission. A separate worker-admission boundary
is required before self-nudge registration can migrate.

The canonical callback URL is exposed in `guardian/status`. Existing explicit
1619 registrations remain readable and retryable during migration.

### Phase 3 — stop creating 1619 by default

Implemented in source for the artifact-only receiver. `scripts/run-local.sh` and
the generated app launcher no longer start `cmd/guardian` by default; the server
owns recovery markers through the canonical route when `-guardian-state-dir` is
configured.

Keep `cmd/guardian` as a compatibility executable for one release cycle or until
registered 1619 callbacks have migrated. Operators that still need the legacy
listener can opt in with:

```text
TIMEKEEPER_GUARDIAN_COMPAT_RECEIVER=1
```

`tk-guardian-self` remains a separate agent-side worker-admission listener; it
is not removed by this phase.

`GET /api/v1/guardian/status` now reports `callback_migration` counters. Treat
`legacy_recovery` as the number of artifact-only `/v1/recover` callbacks that
still require the compatibility receiver; `canonical_recovery` counts exact
`guardian_recovery_url` registrations; and `worker_or_custom` includes `/pulse`
worker-admission callbacks and other custom callback URLs. The counters classify
registration URLs only. They do not prove worker health, worker admission, or
Sprint completion.

### Phase 4 — retire the duplicate listener

After `callback_migration.legacy_recovery` remains zero while the canonical
route is configured and existing canonical callbacks have been observed:

- remove the default 1619 receiver process and launcher code;
- reject new 1619 registration with a structured migration error;
- retain an explicit compatibility flag only if an external deployment still
  needs it;
- update installers, service files, docs, and dashboard status.

## Security requirements for the canonical route

Port consolidation must not mean trust-by-port.

- Validate the exact signal format and action.
- Bind the route to the registered agent identity and nudge ID.
- Enforce a bounded request body and strict content type.
- Use a per-agent callback secret or authenticated local session for recovery
  admission; loopback alone is not authentication.
- Make acknowledgement idempotent.
- Record the recovery artifact before acknowledging.
- Never execute a command from a signal or reminder body.
- For the artifact-only recovery callback, return the acceptance header only
  after the durable artifact is written and the nudge is acknowledged.
- For a future worker-admission callback, return acceptance only after the
  configured adapter establishes its documented admission boundary.
- Preserve the distinction between callback acceptance, durable recovery-marker
  creation, worker admission, and Sprint completion.

## Verification gates

Before removing 1619:

1. Fresh server starts with only 1618 for TimeKeeper.
2. `cmd/guardian -register` registers the canonical 1618 recovery URL when the
   server advertises it, while existing 1619 registrations remain retryable.
3. A synthetic valid signal reaches `/api/v1/guardian/recover` and creates the
   same durable artifact as the legacy receiver.
4. Invalid format/action/agent/nudge/body cases are rejected identically.
5. `guardian/status` reports zero `callback_migration.legacy_recovery` while the
   canonical route remains configured.
6. Repeated delivery acknowledges idempotently without duplicate work.
7. The Hermes adapter still reaches the gateway and observes `source=builtin`.
8. A reference non-Hermes adapter passes the same admission contract.
9. Windows/WSL startup no longer needs a separate 1619 artifact receiver.
10. `tk doctor`, dashboard status, installer output, and docs report one
    canonical TimeKeeper port.
11. The old 1619 compatibility path is tested until its removal release.

## User-facing result

The intended first-use experience becomes:

```text
Use TimeKeeper in terminal.
```

The agent uses the canonical TimeKeeper API on 1618. If it runs inside another
sandbox, its adapter crosses that runtime's boundary; the user does not need
to know whether that boundary is a process, gateway, Unix socket, named pipe,
or authenticated tunnel.
