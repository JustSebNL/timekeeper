# Guardian Control-Plane and Port Consolidation

Status: architecture decision and migration plan; implementation is staged to
avoid breaking existing agents. P-10053/T-10473 tracks the remaining migration
and compatibility checks.

## Decision

TimeKeeper should expose one canonical control-plane listener:

```text
127.0.0.1:1618  TimeKeeper API, dashboard, Pulse, Guardian, recovery routes
```

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
                    127.0.0.1:1618
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

The Windows Core service is separate from any agent-side worker. The current installation uses NSSM; the checked-out installer now supports Bend2Will's `obey` service manager as a replacement while retaining `LocalService`. The installed service has not been migrated in this workspace; the migration and health check must run before describing Bend2Will as deployed.

```text
TimeKeeper Core service (NSSM before migration / Bend2Will after verified migration; delayed start, LocalService)
  -> timekeeper.exe
  -> API + dashboard + in-process Pulse Guardian
  -> 127.0.0.1:1618
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

- Linux uses systemd. The Windows installer can use Bend2Will `obey` with `LocalService`, delayed startup, SCM recovery, and bounded logs; NSSM remains a fallback and the existing NSSM installation is not replaced until `-MigrateFromNssm` is explicitly approved and its health check passes.
- The agent runtime supervises its own adapter where possible.
- If an agent needs a Windows-side watcher, install a separate per-agent
  supervisor under the agent's user context, not as the Core `LocalService`
  account. NSSM may be used for that second process only when its account and
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
4. POSTs nudge acknowledgement back to `127.0.0.1:1618`;
5. returns `X-Timekeeper-Pulse-Accepted: v1`.

The core server already owns the database and exposes Guardian status, Pulse,
agent progress, nudge history, and acknowledgement routes. Maintaining a
second listener creates extra startup, registration, failure, and port-conflict
states without creating a useful security boundary: both endpoints are local
loopback surfaces and the receiver still calls Core for acknowledgement.

## Migration phases

### Phase 1 — additive canonical route

Extract receiver validation/action code from `internal/guardian/receiver.go`
into a reusable handler/service that accepts explicit dependencies:

- private state directory/artifact writer;
- nudge acknowledgement function;
- signal size and schema validation;
- acceptance header/JSON response;
- structured logging.

Mount the handler on the canonical API as:

```text
POST /api/v1/guardian/recover
```

Keep the existing `/v1/recover` receiver route as a compatibility wrapper.
Add tests proving both routes have identical validation and acknowledgement
semantics.

### Phase 2 — canonical registration

Change `tk init` and the built-in self receiver registration to advertise:

```text
http://127.0.0.1:1618/api/v1/guardian/recover
```

Expose the canonical callback URL in `guardian/status`. Existing explicit
1619 registrations remain readable and retryable during migration.

### Phase 3 — stop creating 1619 by default

Remove automatic `tk-guardian-self` startup from the default initialization
path. Keep `cmd/guardian` as a compatibility executable for one release cycle
or until registered 1619 callbacks have migrated. Its status must clearly say
that it is legacy compatibility mode.

### Phase 4 — retire the duplicate listener

After migration telemetry shows no active 1619 registrations:

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
- Return the acceptance header only after the configured adapter has established
  the documented admission boundary.
- Preserve the distinction between callback acceptance, worker admission, and
  Sprint completion.

## Verification gates

Before removing 1619:

1. Fresh server starts with only 1618 for TimeKeeper.
2. `tk init` registers the canonical 1618 recovery URL.
3. A synthetic valid signal reaches `/api/v1/guardian/recover` and creates the
   same durable artifact as the legacy receiver.
4. Invalid format/action/agent/nudge/body cases are rejected identically.
5. Repeated delivery acknowledges idempotently without duplicate work.
6. The Hermes adapter still reaches the gateway and observes `source=builtin`.
7. A reference non-Hermes adapter passes the same admission contract.
8. Windows/WSL startup no longer needs a separate 1619 process.
9. `tk doctor`, dashboard status, installer output, and docs report one
   canonical TimeKeeper port.
10. The old 1619 compatibility path is tested until its removal release.

## User-facing result

The intended first-use experience becomes:

```text
Use TimeKeeper in terminal.
```

The agent uses the canonical TimeKeeper API on 1618. If it runs inside another
sandbox, its adapter crosses that runtime's boundary; the user does not need
to know whether that boundary is a process, gateway, Unix socket, named pipe,
or authenticated tunnel.
