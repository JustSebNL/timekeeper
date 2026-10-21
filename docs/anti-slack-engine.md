# Anti Slack Engine

Friendly name: **Anti Slack Engine**  
Technical name: **Pulse Guardian + agent-side wake adapter**

Status: source implementation complete for FASE identity, state-aware Guardian,
claims, attribution, profiles, and trusted resume; installed-runtime rollout and
runtime-specific policy still require separate verification.

FASE execution is part of the ASE boundary. FASE-ID is attribution, not a login
credential. Core validates the FASE key plus opaque activity session for claims,
resume, completion, and event attribution. A Guardian ACK proves adapter
acceptance only; it does not prove work or renew progress.

Execution claims protect Sprint/parent-Task start, resume, and completion.
Notes, evidence, child work, and other permitted contributions remain
collaborative. Jev may recommend routing or a bounded Sprint, but cannot mutate,
claim, release, or override work; deterministic TimeKeeper policy must revalidate
the current tree, parentage, status, actor, and idempotency first.

The Anti Slack Engine is an optional attention and recovery path for a worker
that has registered a progress lease and a compatible local adapter. TimeKeeper
owns the durable attention state; the agent runtime owns the actual work. The
engine can deliver a nudge, but it cannot guarantee that a runtime resumes or
that a Sprint is completed. It is a protocol, not an LLM or a background
conversation resurrector.

## What it does

1. An agent registers a material-progress lease with TimeKeeper.
2. The agent renews that lease while making real progress.
3. If the lease expires, Pulse Guardian creates a durable pending nudge.
4. TimeKeeper delivers the nudge through a loopback callback and/or ntfy.
5. The agent-side adapter starts or resumes a bounded work cycle.
6. The work cycle inspects the project, acts on one concrete Sprint, verifies
   the result, and records evidence in TimeKeeper.
7. The adapter reports success only after the worker reaches the configured
   verified-continuation boundary; seeing a process in `running` state alone is
   not enough.
8. The worker inspects, verifies, and records the Sprint result; failures keep
   the next nudge/recovery state visible.

A callback that only prints a reminder is not a successful recovery. The
Guardian requires HTTP response header `X-Timekeeper-Pulse-Accepted: v1` before
counting callback delivery as accepted.

## Architecture

```text
TimeKeeper API :1618 (canonical control-plane port)
  |
  +-- Pulse Guardian (in-process)
  |     watches agent_pulse_progress leases
  |     creates pulse_nudges and pulse_events
  |     retries unaccepted deliveries
  |
  +-- Guardian recovery route :1618/api/v1/guardian/recover
  |     validates signal
  |     writes a durable local recovery artifact
  |     acknowledges the nudge only after artifact write
  |
  +-- ntfy notification -> supervised ntfy subscriber
  |                          runs `tk guardian wake --agent <id> --adapter <trusted-adapter>`
  |
  +-- durable fallback -> `tk guardian wake --agent <id> --adapter <trusted-adapter> --poll`

Agent runtime / adapter
  -> inspect `tk pulse`, `tk summary`, `tk tree`, and recent ToT messages
  -> start or resume one bounded Sprint
  -> do work and verify it
  -> record evidence with `tk msg`
  -> complete the Sprint only when evidence exists
  -> renew the lease or leave the next nudge pending
```

The legacy standalone receiver on `127.0.0.1:1619` is compatibility-only and
is being retired. Runtime gateways such as Hermes may have their own port; that
is an adapter boundary, not a second TimeKeeper control-plane port.

## Optional first use for an agent

Ordinary project tracking does not require ASE or a background receiver. Use
this setup only when unattended attention recovery is wanted and an adapter
exists for the intended runtime. Use a stable agent identifier; do not generate
a new one on every wake. On identity-aware CLI builds, `tk agent identity`
shows the resolved FASE ID. It is attribution metadata, not authentication.

```bash
# Confirm the canonical TimeKeeper service is available
tk doctor

# Optional local receiver/lease setup
tk init --agent <agent-id> --project <project-id> --lease 30m
```

`tk init` writes a local `.tkrc`, registers the lease, and attempts to start the
self-nudge receiver on `127.0.0.1:1619`. It does not replace the OS service or
create a second TimeKeeper server, and it does not attach to an arbitrary
running conversation. The receiver needs a trusted adapter that can actually
resume the chosen runtime; see [Agent Wake Adapter Protocol](agent-wake-adapter-protocol.md).

For a worker runtime that already owns its process lifecycle, register through
the API or the progress command and run the durable poll worker separately:

```bash
# Register/renew the agent lease and optionally associate a Sprint
tk agent progress <agent-id> 30m <sprint-id> http://127.0.0.1:1619/pulse

# Durable fallback: retrieve pending nudges and run an explicit work adapter
# The loop is intentionally external to the TimeKeeper server.
tk guardian wake --agent <agent-id> --project <project-id> --adapter <trusted-adapter> --poll --interval 30s
```

A one-shot wake is useful for a supervised notification callback:

```bash
tk guardian wake --agent <agent-id> --project <project-id> --adapter <trusted-adapter>
```

The command reads pending nudges from TimeKeeper, records delivery, runs the
configured work hook, acknowledges only on success, and keeps a bounded agent
context cache between cycles.

## The work adapter is mandatory

The adapter boundary is intentionally agent-neutral. See
`docs/agent-wake-adapter-protocol.md` and copy
`scripts/guardian/agent-wake-adapter.example.sh` for a Claude, Codex, OpenCode,
or custom-runtime integration. Hermes is only one implementation of the
contract; TimeKeeper Core does not depend on Hermes APIs.

```text
scripts/guardian/self-nudge-work.sh
```

When the repository self-nudge shell hook is explicitly selected, it uses the
repository's `hermes-wake-adapter.sh` by default. Direct `tk guardian wake`
commands do not select a runtime implicitly. The Hermes adapter uses the local
gateway as a tunnel: it queues the supervised
`TimeKeeper Pulse tunnel` cron job through the authenticated loopback API and
waits for that exact `source=builtin` execution to complete successfully. It
never nests a second Hermes process inside the current agent sandbox. The cron
worker must inspect durable state, make verified progress, and record evidence;
`HERMES_WAKE_TIMEOUT` bounds the full verified continuation wait. Set
`HERMES_WAKE_JOB_ID` when the local profile uses a different tunnel job, or
replace the adapter with an explicit `AGENT_WAKE_COMMAND` executable.

The adapter receives the reminder as its first argument. Its job is to launch
or resume the agent runtime, not to fake completion:

```bash
#!/usr/bin/env bash
set -euo pipefail
reminder="${1:?missing reminder}"

# Replace this with the local agent runtime's documented resume command.
exec /opt/my-agent/bin/resume-worker --prompt "$reminder"
```

Configure it in the environment of the wake worker when overriding the built-in
Hermes adapter:

```bash
export AGENT_WAKE_COMMAND=/opt/my-agent/bin/resume-worker-adapter
export TK_GUARDIAN_SELF_WORK=/path/to/TimeKeeper/scripts/guardian/self-nudge-work.sh
```

The adapter must return non-zero when it could not establish verified worker
continuation. A `source=builtin`, `running` observation is only intermediate
evidence. The built-in tunnel uses a per-agent lock, coalesces overlap without
acknowledging the nudge prematurely, and bounds the verified continuation wait
with `HERMES_WAKE_TIMEOUT` (default 1800 seconds).

Hermes `/goal` is an optional implementation detail, not part of the Guardian
protocol. The protocol carries a runtime-neutral actionable continuation intent;
Claude, Codex, OpenCode, and other runtimes map it to their own resume or goal
mechanisms while preserving the bound scope, provenance, safety constraints,
and bounded verification requirement.

The Windows/Unix consumer supports bounded continuous mode: after the first
verified wake cycle it may immediately run additional adapter cycles up to a
configured maximum. This is what turns a reminder into actual continued work;
the limit and durable evidence requirements prevent a runaway model loop.
For full queue draining, `--until-idle` re-arms another bounded batch while
TimeKeeper reports Open or Active Sprints, then stops at durable idle state.

## Durable Windows supervision

Do not run the poller only from the interactive agent terminal in production;
that makes ASE disappear when the terminal/session exits. Install the
agent-side worker in the agent user's Windows Task Scheduler context:

```powershell
install\guardian-poller.bat -Action install -Agent example-agent -MaxCycles 4
install\guardian-poller.bat -Action start -Agent example-agent
install\guardian-poller.bat -Action status -Agent example-agent
```

The installer runs `wsl.exe` with the Windows-built `tk.exe`, uses
`--continuous --until-idle`, and configures restart-on-failure. It intentionally
uses the interactive agent identity rather than the LocalService account that
owns the Core service, because Hermes/profile credentials and the WSL
runtime belong to the agent user. Task Scheduler registration may require an
elevated PowerShell session on locked-down Windows installations; an
`Access is denied` result means the poller is not yet durable and must not be
reported as installed.

## What a resumed worker must do

Every wake cycle should be bounded and evidence-producing:

```text
1. tk doctor
2. tk pulse
3. tk summary <project-id>
4. tk tree <project-id>
5. retrieve the latest relevant ToT checkpoint/messages
6. choose one concrete Sprint; do not invent completion
7. make the smallest useful change
8. run the relevant test/build/reproduction check
9. record result, blocker, or handoff with tk msg
10. complete the Sprint only with verification evidence
11. renew progress for the next real action
```

The durable context rule is simple: carry compact `M-<id>` references between
runs, and retrieve the full post when its details matter. Do not depend on a
conversation transcript surviving a process restart.

A useful handoff message is short enough for the 1000-character ToT limit:

```bash
tk msg P-12345 add --kind observation --author example-agent \
  "SP-12346: adapter started; source check and focused tests pass. Next: verify callback acceptance and poll retry. Full evidence: dev-ops/<file>.md"
```

## Nudge signal contract for other agents

A custom callback receives `POST` JSON with this shape:

```json
{
  "format": "timekeeper-pulse-guardian/v1",
  "action": "recover_attention",
  "reminder": "You're idling — please continue remaining work.",
  "nudge": {
    "nudge_id": 123,
    "agent_id": "example-agent",
    "kind": "agent_unresponsive"
  }
}
```

The callback must:

- bind to numeric loopback unless the deployment has an explicitly reviewed
  authenticated transport;
- validate `format`, `action`, `nudge_id`, and `agent_id`;
- start or signal the agent adapter;
- return a success status only when the nudge has been accepted for processing;
- include `X-Timekeeper-Pulse-Accepted: v1` on that success response;
- return a non-2xx response when it cannot accept the nudge;
- never execute arbitrary commands from the request body;
- never acknowledge a failed work cycle.

The durable acknowledgement endpoint is:

```text
POST /api/v1/agents/{agent-id}/nudges/{nudge-id}/ack
Content-Type: application/json
{}
```

The agent can inspect pending work with:

```text
GET /api/v1/agents/{agent-id}/nudges
```

It can record lifecycle evidence at:

```text
POST /api/v1/agents/{agent-id}/nudges/{nudge-id}/events
{"event_type":"delivered"}
```

Use the documented CLI where possible; direct API callers must preserve the
same ordering and retry semantics.

## ntfy transport

Pulse Guardian can publish a wake notification through ntfy when configured.
The ntfy subscriber is a transport process, not the worker itself. Configure it
to invoke:

```bash
tk guardian wake --agent example-agent --project P-12345 --adapter /path/to/adapter
```

The pending nudge remains in TimeKeeper, so a missed ntfy notification is not
data loss. Keep the poll worker as the last-resort recovery path:

```bash
tk guardian wake --agent example-agent --project P-12345 --adapter /path/to/adapter --poll --interval 30s
```

Use one primary wake adapter per agent. Running several independent workers
against the same agent can create duplicate work; if multiple transports are
needed, they must converge on one idempotent wake handler.

## Lease discipline

A lease is evidence that the agent is actively making material progress; it is
not a permanent heartbeat. Renew it when starting or finishing a concrete
step, and include the active Sprint when possible:

```bash
tk agent progress example-agent 20m SP-12346 http://127.0.0.1:1619/pulse
```

Choose a lease longer than the expected verification step but short enough that
an abandoned worker is noticed. A lease that is refreshed without actual work
just teaches the engine that slacking is progress. The Anti Slack Engine is
not fooled by optimistic timestamps forever.

## Troubleshooting

### No wake arrives

```bash
tk doctor
tk agent status example-agent
tk agent nudges example-agent
curl -sS http://timekeeper.local/api/v1/guardian/status
```

Then check:

- the agent ID is stable and registered;
- `guardian_url` is reachable from the TimeKeeper process;
- the receiver is bound to the expected loopback address;
- ntfy subscriber or poll worker is running;
- the work adapter is executable;
- the nudge is still `Pending` rather than already acknowledged.

### Nudge repeats

This is normally correct when delivery or work failed. Inspect the adapter exit
status and callback response. A callback must return the acceptance header, and
the work adapter must not hide errors.

### A nudge was acknowledged but no work happened

Treat this as a bug. The adapter acknowledged too early or a stale receiver is
running. Stop the stale process only after identifying it, restore the nudge
path, and record the incident in the owning project's ToT board.

### Context was lost

Do not reconstruct the project from chat. Run `tk doctor`, inspect the project
tree/summary, retrieve the latest `M-<id>` checkpoint, and continue from the
next unverified Sprint.

## Security boundary

- Keep the callback on numeric loopback by default.
- Do not expose port 1619 to a LAN or public network without authentication and
  an explicit threat review.
- Treat agent-provided headers and identifiers as diagnostic metadata, not
  authentication.
- Do not put credentials, bearer tokens, private prompts, or unbounded logs in
  nudge bodies or ToT messages.
- The receiver may launch only a configured local adapter; request payloads do
  not choose commands.

## Implementation map

| Surface | Location |
|---|---|
| Pulse Guardian sender | `internal/guardian/guardian.go` |
| Durable receiver contract | `internal/guardian/receiver.go` |
| Cross-platform wake CLI | `internal/cli/guardian_wake.go` |
| Bootstrap command | `internal/cli/tk_init.go` |
| Loopback receiver binary | `cmd/tk-guardian-self/main.go` |
| Conservative default hook | `scripts/guardian/self-nudge-work.sh` |
| Stable pulse monitor | `scripts/guardian/timekeeper-pulse-monitor.sh` |
| Existing design reference | `docs/guardian-self-nudge.md` |
| Agent first-use workflow | `docs/HARNESS.md` and `harness/SKILL.md` |

The technical design remains in `docs/guardian-self-nudge.md`; this document is
the operator and agent usage guide for the Anti Slack Engine.
