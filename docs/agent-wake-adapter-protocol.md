# Agent Wake Adapter Protocol

Status: stable adapter contract; transport implementations are agent-specific.

TimeKeeper does not embed a Claude, Codex, Hermes, OpenCode, or custom-agent
launcher. Pulse Guardian calls one trusted executable through the
`AGENT_WAKE_COMMAND` boundary. That executable is the agent-side adapter.

This keeps the TimeKeeper Core stable while allowing each agent runtime to
bridge its own sandbox, gateway, supervisor, or resume API.

## Portable CLI setup

Configure a trusted adapter explicitly. The CLI no longer implicitly selects
Hermes or requires a repository-relative Bash hook:

```powershell
tk agent progress my-agent 30m SP-12345
tk guardian wake --agent my-agent --project P-123 --adapter 'C:\Agent Tools\wake.ps1' --poll --continuous --until-idle --max-cycles 4 --work-timeout 30m
```

```sh
tk guardian wake --agent my-agent --project P-123 --adapter /opt/my-agent/wake --poll --continuous --until-idle --max-cycles 4
```

`--adapter` overrides `AGENT_WAKE_COMMAND`; both select one executable path,
not a command string. Put runtime arguments in an adapter script. The legacy
`TK_GUARDIAN_SELF_WORK` hook remains available when neither is supplied. If that
variable explicitly selects the repository's `self-nudge-work.sh`, that shell
hook uses the built-in Hermes adapter by default; the direct CLI path does not.
Existing Hermes installations can also select
`scripts/guardian/hermes-wake-adapter.sh` explicitly. With no adapter or legacy
hook configured, the wake fails with the nudge pending rather than implicitly
launching another runtime.

Project binding comes from `--project`, the same agent's saved context, or a
matching `.tkrc` agent/project binding. Before launch the CLI checks the project
execution tree and the pending nudge's Sprint ownership and active status. It
passes the **selected pending** nudge ID and Sprint, canonical project ID, and
actual API URL to the adapter. Stale or mismatched scope fails closed.

Agent IDs use 1–128 letters, digits, dots, underscores or hyphens (excluding
`.` and `..`). Use a stable identifier independent of a runtime's display name.
Native executables launch directly; `.ps1` uses noninteractive PowerShell;
`.sh` uses Bash (WSL on Windows), and Windows `.bat`/`.cmd` uses cmd. Native and
PowerShell adapters do not need WSL. Adapter waits default to 30 minutes and
can be bounded with `--work-timeout` (positive, at most 24 hours). A timeout
leaves the nudge pending; the supervisor must still own/cancel any descendants
and deduplicate retries, because killing the adapter is not proof its worker stopped.

The CLI verifies the acknowledgement response before recording acknowledgement
or incrementing completed cycles. API outages are errors, not an idle queue.
`--until-idle` checks durable queue state before every additional cycle and works
in both polling and single-shot mode.

This is a common integration boundary, not a universal vendor session API.
Each runtime still needs a trusted bridge to its supported resume/job mechanism.
Neither installing `tk` nor registering a lease attaches to an arbitrary running
desktop conversation. The checked-in native/PowerShell tests prove transport,
scope, failure, and acknowledgement behavior; verify the actual runtime's work
receipt before claiming that runtime is integrated.

For an explicitly installed Windows poller, the scheduled-task installer accepts
`-Agent my-agent -Project P-123 -Adapter 'C:\Agent Tools\wake.ps1'`. That mode runs
the installed Windows `tk.exe` directly; without `-Adapter` it retains the
existing WSL deployment mode. Install a CLI containing these flags before using
the new installer mode. Task installation/start is separate from source build
verification and must use the intended runtime's adapter.

## Contract

The adapter is invoked as:

```text
AGENT_WAKE_COMMAND <reminder>
```

The first argument is advisory text only. It is not a shell command and must
never be evaluated or interpolated into a command line.

The adapter receives these environment variables when available:

| Variable | Meaning |
|---|---|
| `AGENT_ID` | Stable TimeKeeper agent identity |
| `AGENT_LAST_NUDGE_ID` | Numeric pending nudge selected for this cycle |
| `AGENT_LAST_PROJECT_ID` | Canonical project associated with the nudge |
| `AGENT_LAST_SPRINT_ID` | Sprint associated with the nudge, if any |
| `AGENT_WORK_CYCLES` | Number of completed wake cycles known to the client |
| `PROJECT_ID` | Alias of `AGENT_LAST_PROJECT_ID` for runtimes that expect it |
| `TK_BASE_URL` | TimeKeeper API base URL |
| `TIMEKEEPER_STATE_DIR` | Local runtime state directory, when configured |

The adapter may use the TimeKeeper CLI/API to retrieve the full durable nudge.
It must not trust the reminder as an instruction payload.

## Exit semantics

- Exit `0` only after the adapter has evidence that a real worker execution
  was accepted by the agent runtime or its supervisor and reached the
  adapter's verified continuation boundary. For the TimeKeeper harness, this
  means the worker had an opportunity to inspect durable state, perform one
  bounded step, verify it, and record evidence.
- Exit non-zero when the runtime is unavailable, authentication fails, an
  overlapping execution exists, or admission cannot be verified.
- Do not return success merely because a reminder was printed or a process was
  forked without an ownership/admission receipt.
- Admission is not Sprint completion. The worker must later verify its work,
  record evidence with `tk msg`, renew its lease, and leave a durable blocker if
  it cannot proceed.

TimeKeeper acknowledges the wake after the adapter's verified work boundary.
The agent runtime owns execution and is responsible for subsequent progress.

## Meaning of a trusted tunnel message

After the Guardian signal has passed the registered-agent, nudge, transport,
and adapter checks, the runtime should treat delivery as an actionable
continuation instruction. In practical terms it is equivalent to a user saying
“continue the already-authorized TimeKeeper work”; the worker should not wait
for another chat turn before rehydrating state and acting.

The equivalence is scoped, not unrestricted authority. A tunnel signal may
resume the bound Project/Task/Sprint and request one bounded continuation step,
but it cannot override user constraints, security policy, approval gates,
project ownership, or runtime safety rules. Reminder text is data, never shell
syntax or an arbitrary command channel. The adapter and fixed job contract
define the allowed action.

## Runtime-neutral continuation intent

The protocol does **not** standardize a Hermes command such as `/goal`. That
would make Guardian depend on one agent runtime. The stable contract is the
meaning of the signal:

```text
event: guardian.continuation
priority: actionable
provenance: authenticated-timekeeper-guardian
scope: bound project/task/sprint from the durable wake context
objective: continue already-authorized work
budget: one bounded verified continuation step
```

Each adapter translates that intent into its runtime's native mechanism:

- Hermes may use a cron prompt or its session-backed goal loop.
- Claude, Codex, OpenCode, or another runtime may inject an equivalent user
  message, resume a named session, call its supervisor, or use its own goal
  primitive.
- A runtime without persistent goals must still perform the fixed continuation
  loop from the harness; it must not ignore the signal merely because it lacks
  a `/goal` command.

The adapter must preserve the semantic fields and provenance while translating;
it must not pass arbitrary nudge text as executable commands. This keeps the
Guardian/Core contract stable while allowing runtime-specific implementations.

## Bounded continuous mode

An agent-side consumer may request continuous mode for a wake. This is a
bounded worker run, not an unbounded daemon and not a permission to spend
indefinitely:

- the first successful cycle owns and acknowledges the nudge;
- follow-up cycles carry the same continuation intent and increment a durable
  cycle counter;
- the adapter stops at its configured maximum, on a runtime failure, or when
  the worker reports a blocker/no-runnable-work condition;
- every cycle must renew/report meaningful progress and leave evidence in
  TimeKeeper;
- another worker's exit-75 coalescing result is not a completed cycle.

The Core default remains bounded and configurable. A runtime may implement a
long-lived native goal loop internally, but it must retain the same stop,
scope, provenance, and evidence rules.

For a project worker that must continue without another chat message, the
consumer may use `--continuous --until-idle --max-cycles N`. Each batch remains
bounded, but the consumer automatically starts the next batch while the
durable project operational summary still reports Open or Active Sprints. It
stops at durable project idle state, adapter failure, or the hard safety cap.

## Single-flight and restart safety

Every adapter must:

1. Use a stable agent identity; do not generate a new identity per wake.
2. Enforce one active execution per agent, preferably with a local lock plus a
   runtime-side idempotency key based on `AGENT_LAST_NUDGE_ID`.
3. Be safe to retry after a process, WSL session, or gateway restart.
4. Bound the admission wait. A long worker task must continue under its
   supervisor rather than holding the Guardian callback open indefinitely.
5. Report the distinction between queued, running, completed, and unknown.
6. Leave the nudge pending when admission cannot be proven.

Adapters may use exit code `75` to report a coalesced wake: another worker for
the same stable agent identity owns the nudge. This is not completion. The
consumer must leave the nudge pending unless the owning worker separately
confirms successful continuation/completion. Any other non-zero exit code
means admission or continuation was not proven and leaves the nudge eligible
for retry. This convention is transport neutral and works for `.sh`, `.bat`,
PowerShell, and native executable adapters.

## Transport choices

The protocol does not require a particular transport. Use the narrowest
trusted bridge available:

- Same host, same runtime: invoke the agent's documented resume command.
- Separate process or sandbox: authenticated loopback HTTP/Unix socket/ named
  pipe to the agent supervisor.
- Gateway-managed agent: submit a durable job to the gateway scheduler and
  wait for a gateway-owned execution receipt.
- Remote agent host: use an authenticated, encrypted control channel with
  destination and agent identity binding; never expose an unauthenticated
  wake endpoint.
- Push notification: use ntfy, webhook, or another broker only when the
  receiver can recover the durable nudge from TimeKeeper and provide a real
  admission result.

A public tunnel is not required when both sides share a trusted host gateway.
If the agent runtime is isolated behind another host or network namespace, the
same contract applies over a mutually authenticated tunnel.

## Reference shell adapter

Copy this as the starting point for a local or gateway-backed runtime:

```bash
#!/usr/bin/env bash
set -euo pipefail

REMINDER="${1:-TimeKeeper requested a worker wake.}"
AGENT_ID="${AGENT_ID:?AGENT_ID is required}"
NUDGE_ID="${AGENT_LAST_NUDGE_ID:-unknown}"
LOCK_DIR="${XDG_RUNTIME_DIR:-/tmp}/timekeeper-wake-${AGENT_ID}.lock"

if ! mkdir "$LOCK_DIR" 2>/dev/null; then
    printf 'wake adapter: agent %s already has an active execution\n' "$AGENT_ID" >&2
    exit 75
fi
trap 'rmdir "$LOCK_DIR" 2>/dev/null || true' EXIT

# Replace this block with the agent runtime's authenticated supervisor call.
# The supervisor must deduplicate by AGENT_ID + NUDGE_ID and return an
# admission receipt. Do not execute REMINDER as shell syntax.
/opt/agent-runtime/bin/submit-wake \
    --agent "$AGENT_ID" \
    --nudge "$NUDGE_ID" \
    --reason "$REMINDER"
```

For a gateway adapter, replace only the `submit-wake` block with an
authenticated request to the gateway. Keep the lock, stable IDs, bounded wait,
and admission verification unchanged.

Do not use `exec` in the shell reference: the EXIT trap must run to release its
lock after the supervisor returns. An unknown leftover shell lock requires
ownership inspection before removal. Windows callers can start from
`scripts/guardian/agent-wake-adapter.example.ps1`, which uses an exclusive file
handle released automatically on process exit. Both templates require a real
`AGENT_RUNTIME_SUBMIT` supervisor; they do not implement a vendor API. The
supervisor should deduplicate retries using agent + nudge + continuation cycle,
so a new authorized cycle is distinguishable from replaying the same wake.

## Hermes implementation

The repository's Hermes adapter is an implementation of this contract:

```text
scripts/guardian/hermes-wake-adapter.sh
  -> scripts/guardian/hermes-cron-tunnel.py
  -> authenticated loopback /api/jobs/{job_id}/run
  -> Hermes gateway scheduler
  -> source=builtin cron execution
```

The Python bridge resolves `API_SERVER_KEY` through Hermes's profile-scoped
secret resolver. It never prints or stores the key. The adapter waits for the
gateway-owned run's successful completion boundary; queued or running alone
does not establish verified continuation.

Configure a different installation with:

```bash
export AGENT_WAKE_COMMAND=/absolute/path/to/your-agent-wake-adapter
```

For a different Hermes profile/job:

```bash
export HERMES_WAKE_JOB_ID=<durable-job-id>
```

Do not put secrets, bearer tokens, or arbitrary command text in the nudge body.
Use the runtime's secret store and bind authorization to the configured agent
identity.

## Platform notes

- Linux/WSL `.sh` adapters run through Bash.
- Windows `.sh` adapters are routed through `wsl.exe -- bash <absolute-path>`.
- Windows `.bat` adapters run through `cmd.exe /c`.
- Use absolute paths or resolve paths relative to the adapter itself; do not
  depend on the caller's current directory.
- Do not require an interactive terminal. Background Guardian execution has no
  TTY and must not invoke interactive login or approval flows.

## Verification checklist

Before declaring an adapter integrated:

1. `bash -n` or the platform equivalent passes.
2. Missing runtime/authentication returns non-zero and leaves the nudge pending.
3. A duplicate wake is rejected without creating a second worker.
4. A real gateway/supervisor receipt is observed before exit `0`.
5. The worker can continue after the adapter process exits.
6. Restarting the adapter does not lose the durable nudge.
7. The worker records a `tk msg` evidence or blocker message.
8. The agent lease is renewed during real progress.
9. Logs contain no credentials, bearer tokens, or unrestricted reminder payloads.
10. Test both a same-host runtime and the actual sandbox/gateway boundary used
    in production.
