# How the Current Anti Slack Engine Gets Input to Another Developer/Agent

Status: current implementation
Audience: developers integrating or debugging a watched agent runtime

Deployment-specific note: the `xatia` identity and polling command below refer to
this checkout's configured agent setup. They are not general installation
instructions. For a new runtime, follow the [portable adapter guide](agent-wake-adapter-protocol.md).

Portable CLI update (2026-09-26, source implementation): `tk guardian wake`
accepts `--project`, `--adapter`, and `--work-timeout`. Native executables and
PowerShell adapters run without Bash/WSL; `.sh` remains supported. The CLI checks
the pending nudge's project/Sprint binding and acknowledgement response. Use the
[portable setup guide](agent-wake-adapter-protocol.md#portable-cli-setup).
The Hermes deployment below is deployment context; these source changes do
not install an adapter or replace a running poller.

This document explains the actual path from “the agent has stopped making material progress” to “the other developer/agent receives an actionable continuation signal and resumes authorized work.” It describes the current TimeKeeper implementation, not an imagined universal agent API.

The short version is:

```text
agent reports a material-progress lease
        |
        v
lease expires or the active Sprint becomes overdue
        |
        v
TimeKeeper Pulse Guardian creates a durable Pending nudge
        |
        +--> optional ntfy wake message
        |
        +--> registered numeric-loopback callback
        |
        +--> agent-side polling fallback
                    |
                    v
             tk guardian wake
                    |
                    v
             trusted wake adapter
                    |
                    v
             runtime-specific supervisor/gateway
                    |
                    v
             another developer/agent receives actionable continuation input
                    |
                    v
             worker rehydrates TimeKeeper and performs bounded work
                    |
                    v
             verification/evidence/progress renewal
                    |
                    v
             nudge acknowledgement and automatic continuation while work remains
```

The key design decision is that the reminder is not a passive notification. Once it has passed the trusted Guardian and adapter boundaries, its meaning is:

```text
Continue the already-authorized TimeKeeper work now.
Do not wait for another user message.
```

That meaning is deliberately narrower than unrestricted user authority. The signal can resume the bound project/task/Sprint, but it cannot bypass approvals, security policy, ownership, secrets policy, deployment gates, or spending controls.

## 1. The components and their ownership

There are several processes. They must not be confused with one another.

### 1.1 TimeKeeper Core and Store

TimeKeeper Core owns durable control-plane state:

- agent identities;
- material-progress leases;
- active Sprint association;
- nudge records;
- nudge delivery attempts;
- acknowledgement state;
- Pulse attention/deadline calculations;
- project/Sprint operational summaries;
- evidence and blocker messages.

The Store is the authority for whether a wake is warranted. A terminal being quiet, a process existing, or a poller printing a heartbeat is not enough to prove that the agent is working.

### 1.2 Pulse Guardian evaluator

The evaluator runs from the TimeKeeper server/control plane. `internal/guardian/guardian.go` owns the periodic `Tick` and `Run` loop. The Store implementation in `internal/store/pulse_guardian.go` owns the durable evaluation logic.

The evaluator is outside the watched agent's work loop. This is important: a hung agent cannot prevent the component responsible for noticing the hang from running.

### 1.3 Agent-side consumer

The agent-side consumer is the `tk guardian wake` CLI path in `internal/cli/guardian_wake.go`.

For the currently configured `xatia` path, the consumer is run as a Windows `tk.exe` process, with WSL used when a shell hook must execute. The current intended long-running command is:

```text
tk guardian wake --agent xatia --poll --continuous --until-idle --max-cycles 4 --interval 30s
```

Its responsibilities are:

1. poll Core for pending nudges;
2. select a pending nudge for the stable agent identity;
3. record delivery evidence;
4. invoke the configured work hook/adapter;
5. acknowledge the nudge only after the work-cycle boundary succeeds;
6. persist bounded agent context;
7. continue additional bounded work while durable project work remains;
8. return to polling instead of waiting for a chat turn.

### 1.4 Runtime adapter

The runtime adapter translates the stable Guardian continuation intent into the mechanism used by a particular agent runtime.

The Core does not contain Hermes, Claude, Codex, OpenCode, or another runtime's internal launcher. The CLI calls a trusted executable selected by `--adapter` or `AGENT_WAKE_COMMAND`, or an explicitly configured legacy `TK_GUARDIAN_SELF_WORK` hook. The legacy shell hook selects Hermes by default; the portable CLI itself does not select a runtime implicitly.

For Hermes, the adapter submits a durable job to the authenticated Hermes gateway. For another runtime, the adapter could call an authenticated local supervisor, resume a named session, inject a runtime-native user-equivalent message, or submit a job to a scheduler.

### 1.5 The worker/developer agent

The runtime worker owns actual project work. It must:

- rehydrate durable TimeKeeper state;
- inspect the authorized Project/Task/Sprint;
- choose a bounded unfinished step;
- implement or investigate that step;
- run relevant verification;
- record evidence or a blocker;
- renew material progress while work continues.

The worker must not interpret the reminder as shell syntax, arbitrary code, or a new source of authority.

## 2. How inactivity is detected

An agent records material progress through the TimeKeeper API. A compatible
`tk`/`timekeeper` command automatically resolves a permanent FASE identity on
first use; there is no required registration command. The important lease data
is stored in `agent_pulse_progress`:

```text
agent_id
active_sprint_id
lease_duration_seconds
guardian_url
last_progress_at
updated_at
```

The legacy lease-compatible API remains:

```http
POST /api/v1/agents/{agent_id}/progress
Content-Type: application/json

{
  "active_sprint_id": "SP-...",
  "lease_duration_seconds": 300,
  "guardian_url": "http://127.0.0.1:1619/v1/recover"
}
```

Fields may be omitted according to the API contract when the caller wants to preserve an existing Sprint or callback association. The lease is not a generic health check. It is an assertion by the agent that it is making meaningful progress on active work and will renew that assertion before the deadline.

The evaluator calls:

```go
Store.EvaluatePulseGuardianAt(ctx, at)
```

It first calculates Pulse attention, including overdue Sprint attention. It then reads registered progress leases and joins the associated Sprint status.

FASE activity adds the identity boundary around this flow. Core maps the
fingerprint to `FASE-<key>` and a month-of-use `FASE-ID.<key>.<month>.<year>`.
Public work-state uses an opaque activity session token. Guardian validates the
FASE agent plus that token before resuming a trusted adapter; the poller's own
identity is never copied to the watched worker.

A nudge can be created when:

```text
current_time - last_progress_at > lease_duration_seconds
```

or when Pulse identifies the associated Sprint as overdue. The nudge kind is normally one of:

```text
agent_unresponsive
sprint_overdue
```

A held or terminal Sprint is not considered active work. This prevents a completed/paused Sprint from generating endless idle reminders. Unknown legacy Sprint IDs retain lease-based compatibility behavior.

The Store creates one `Pending` row using `INSERT OR IGNORE`. Repeated evaluator ticks do not create duplicate Pending nudges for the same underlying agent condition.

The durable nudge contains, among other fields:

```text
nudge_id
agent_id
active_sprint_id
kind
status = Pending
detected_after_seconds
delivery_attempts
last_delivery_at
delivered_at
created_at
acknowledged_at
acknowledged_by
```

The important state rule is:

```text
Pending remains Pending until the owning agent explicitly acknowledges it.
```

Creating a nudge is detection, not proof that another developer/agent received input or did work.

## 3. How the nudge is delivered

There are three delivery mechanisms in the current code. They have different roles.

### 3.1 Direct numeric-loopback callback

If an agent registers a `guardian_url`, Core exposes that nudge through `ListPendingPulseNudgeDeliveries`. The URL is accepted only if it is:

- plain HTTP;
- numeric loopback;
- explicit host and non-zero port;
- free of userinfo, query, and fragment components.

The Guardian sends a JSON signal to that callback:

```json
{
  "format": "timekeeper-pulse-guardian/v1",
  "action": "recover_attention",
  "reminder": "You're idling — please continue remaining work.",
  "nudge": {
    "nudge_id": 123,
    "agent_id": "xatia",
    "active_sprint_id": "SP-...",
    "kind": "agent_unresponsive",
    "status": "Pending"
  }
}
```

The callback must return a successful HTTP response plus:

```text
X-Timekeeper-Pulse-Accepted: v1
```

Without that header, Core treats delivery as unconfirmed.

The HTTP client has a three-second timeout and does not follow redirects. Failed callbacks remain eligible for retry. The delivery-attempt record increments regardless of success, while `delivered_at` is set only after an accepted callback.

The legacy/reference receiver in `internal/guardian/receiver.go` is intentionally constrained. It:

1. accepts only `POST /v1/recover`;
2. limits the body to 64 KiB;
3. validates the signal format and action;
4. validates that the nudge ID and agent ID exist;
5. writes a local recovery artifact;
6. acknowledges through the TimeKeeper API;
7. returns the accepted header only after those operations succeed.

That receiver does not execute arbitrary commands and does not mutate the Store directly. It is a narrow recovery surface.

For the currently configured `xatia` deployment, the canonical active path is the agent-side API poller. The old `1619` callback registration was removed to avoid two independent owners competing for the same wake. The callback mechanism remains part of the runtime-neutral Core contract for other integrations and controlled migrations.

### 3.2 ntfy notification

`guardian.Tick` can optionally publish a wake message through ntfy for each newly created nudge. The ntfy message is only a wake transport. It is not the source of truth and does not replace durable nudge recovery.

The receiving agent must use the nudge ID/agent identity to recover the full durable record from TimeKeeper. If ntfy delivery fails, the durable nudge remains pending and the HTTP/polling path can still deliver it.

### 3.3 Agent-side polling fallback

The `--poll` mode is the important fallback and the current `xatia` path:

```text
GET /api/v1/agents/xatia/nudges
```

The CLI polls at the configured interval, defaulting to 30 seconds. If the API is temporarily unavailable, it logs the failure and retries rather than losing the nudge.

When it sees one or more pending nudges, it selects the first pending nudge, records a `delivered` Pulse event, and invokes the work cycle. It does not wait for another chat response.

Polling is intentionally separate from the worker's actual code/task loop. A worker can be idle while the polling process remains healthy; therefore the poller's liveness is not treated as project progress.

## 4. How the agent-side CLI turns the reminder into input

The entry point is:

```text
tk guardian wake --agent <stable-agent-id> --project <project-id> --adapter <trusted-adapter>
```

The command supports:

```text
--poll
--continuous
--until-idle
--max-cycles 1..32
--interval <duration>
--project <P-ID>
--adapter <executable-path>
--work-timeout <duration>
```

The defaults are one cycle for single-shot mode and four cycles when `--continuous` is selected unless an explicit maximum is supplied.

Before starting work, the CLI loads a bounded local context file:

```text
.timekeeper/state/agent-context-<agent-id>.json
```

The context contains:

```text
agent_id
last_nudge_id
last_nudge_time
work_cycles
last_sprint_id
last_project_id
notes
```

This is not a replacement for the TimeKeeper Store. It is a small continuity cache used to pass stable identity and recent cycle metadata into the runtime adapter. The authoritative nudge, Sprint, project, evidence, and acknowledgement state remains in Core.

The CLI then invokes the configured adapter. An existing Hermes installation
may explicitly retain this legacy work hook:

```text
scripts/guardian/self-nudge-work.sh
```

The hook exports the semantic continuation fields:

```text
GUARDIAN_EVENT=guardian.continuation
GUARDIAN_PRIORITY=actionable
GUARDIAN_OBJECTIVE=continue already-authorized TimeKeeper work
GUARDIAN_SCOPE=bound project/task/sprint from durable wake context
```

The CLI also provides runtime context such as:

```text
AGENT_ID
AGENT_LAST_NUDGE_ID
AGENT_LAST_PROJECT_ID
AGENT_LAST_SPRINT_ID
AGENT_WORK_CYCLES
PROJECT_ID
GUARDIAN_CONTINUATION_MODE
GUARDIAN_CONTINUATION_CYCLE
GUARDIAN_CONTINUATION_BUDGET
```

The reminder is passed as the adapter's first argument for human-readable provenance. It is not interpolated into a shell command. The adapter must treat it as data.

On Windows, the CLI deliberately routes `.sh` hooks through:

```text
wsl.exe -- bash <absolute-path>
```

It uses `cmd.exe /c` for `.bat` hooks. This avoids the failure mode where Windows `cmd.exe` tries to parse a Bash script and reports that `scripts` is not a command.

## 5. How the Hermes adapter obtains the other developer's input

Hermes is the currently implemented runtime adapter. The path is:

```text
self-nudge-work.sh
  -> hermes-wake-adapter.sh
  -> hermes-cron-tunnel.py
  -> authenticated Hermes loopback gateway
  -> durable Hermes cron job
  -> supervised Hermes worker
```

### 5.1 Shell adapter and single-flight lock

`hermes-wake-adapter.sh` first determines the stable agent ID and state directory. It creates:

```text
.timekeeper/state/guardian-wake-<agent-id>.lock/
```

The lock contains the owner PID.

If another live process owns the lock, the adapter exits with code `75`. That means:

```text
another worker already owns this wake; coalesce the duplicate
```

It does not mean Sprint completion, successful work, or permission to acknowledge the nudge. The nudge remains retryable unless the owning worker produces the required evidence.

Dead-owner locks are reclaimed. Unknown/ambiguous locks fail closed rather than allowing two workers to run concurrently.

The adapter also checks the latest Hermes cron run. If the configured durable job is already running, it returns exit `75` and avoids queueing a duplicate gateway execution.

### 5.2 Authenticated gateway bridge

The Python bridge, `hermes-cron-tunnel.py`, does not start a nested interactive Hermes process. That earlier approach was unsafe and produced TTY errors such as:

```text
tcsetattr: Inappropriate ioctl for device
```

Instead, it:

1. loads the Hermes profile-scoped API key through Hermes's secret resolver;
2. never prints or stores the key;
3. inspects the configured cron job through the loopback gateway;
4. records the baseline `last_run_at` value;
5. submits `POST /api/jobs/{job_id}/run` with bearer authentication;
6. polls the gateway-owned run state;
7. distinguishes queued/running/completed/failed states;
8. returns failure if successful completion cannot be verified.

The configured job is currently named:

```text
TimeKeeper Pulse tunnel
```

and is identified by a durable job ID held in the local Hermes configuration. The job prompt tells the Hermes worker that the event is a worker execution, not a conversational answer, and instructs it to inspect durable TimeKeeper state, continue already-authorized work, and not wait for another user reply.

The adapter must not claim success merely because the gateway accepted a request or reported `running`. Those are intermediate states. The worker must be allowed to inspect state, perform a bounded step, verify it, and record evidence.

### 5.3 What the Hermes worker receives

The Hermes worker receives the semantic equivalent of a high-priority user continuation message:

```text
Trusted TimeKeeper Pulse Guardian continuation signal.
This is a worker execution, not a conversational reply.
Continue the already-authorized TimeKeeper work.
Do not wait for another chat reply.
```

The message is scoped by the durable context. The worker should immediately run the project's normal rehydration commands or API equivalents, typically including:

```text
tk doctor
tk pulse
tk summary
tk tree
```

It then identifies the project/task/Sprint already authorized for the agent. If the current Sprint is complete, it may claim/select the next authorized runnable Sprint according to the project rules. It must not invent a new project or broaden authority because it was reminded.

The worker performs one bounded highest-value step, verifies it, and records one of:

- implementation/test evidence;
- a Sprint transition;
- a durable blocker;
- a question requiring human input.

A conversational answer without durable project evidence is not sufficient success.

## 6. The acknowledgement boundary

Acknowledgement is deliberately after work admission and the work-cycle boundary, not before detection.

In the canonical CLI flow:

1. pending nudge is selected;
2. `delivered` event is recorded;
3. the work hook/adapter runs;
4. successful return means the configured worker boundary completed according to the adapter contract;
5. the CLI calls:

```http
POST /api/v1/agents/{agent_id}/nudges/{nudge_id}/ack
Content-Type: application/json

{}
```

6. the CLI records an `acknowledged` event;
7. local agent context is updated;
8. the progress cycle counter is incremented.

The Store's `AcknowledgePulseNudge` operation is transactional. It:

- verifies that the nudge belongs to the claiming agent;
- makes the operation idempotent if already acknowledged by that agent;
- changes status from `Pending` to `Acknowledged`;
- records `acknowledged_at` and `acknowledged_by`;
- renews `last_progress_at` and `updated_at` for that agent.

If acknowledgement fails, the nudge must remain recoverable. A worker that did work but cannot reach Core should not silently erase the durable wake; the next retry can reconcile the state.

The direct legacy receiver follows a slightly different narrow path: it writes a local recovery artifact and then acknowledges through Core. That path proves recovery-signal receipt, not a full project work cycle. For a project worker, the CLI/adapter path is the stronger continuation boundary because it invokes the actual work hook before acknowledgement.

## 7. How continuous work prevents the chatbot-waiting failure

A single wake can be run once, but that does not satisfy the Anti Slack Engine's purpose if the worker returns to a waiting state after one small step.

Continuous mode changes the behavior:

```text
tk guardian wake \
  --agent xatia \
  --poll \
  --continuous \
  --until-idle \
  --max-cycles 4 \
  --interval 30s
```

The first cycle owns and acknowledges the nudge. Follow-up cycles use the same admitted worker and increment the durable local cycle counter. Each cycle must still be bounded and must produce meaningful progress or a blocker.

With `--until-idle`, the CLI checks:

```http
GET /api/v1/projects/{project_id}/operational-summary
```

and reads:

```json
{
  "open_sprints": 0,
  "active_sprints": 1
}
```

If either `open_sprints` or `active_sprints` is greater than zero, the worker can start another bounded batch. If both are zero, it reports durable project idle and returns to the outer poll loop. No new chat message is required.

The implementation retains safety boundaries:

- `--max-cycles` limits cycles per batch;
- `--until-idle` limits continuation by durable project state;
- a hard batch cap prevents malformed state from creating an infinite model loop;
- adapter/runtime failure stops continuation;
- a blocker stops continuation and remains visible;
- exit `75` is coalescing, not completion.

This produces the intended behavior:

```text
not: wake once -> answer/chat -> wait forever

but: wake -> inspect -> work -> verify -> record -> continue while authorized work exists -> stop at durable idle
```

## 8. Failure and retry behavior

### No adapter configured

The default hook fails closed when no executable adapter is configured. The nudge remains pending. This is preferable to claiming that work happened when no runtime received it.

### Adapter authentication failure

The adapter exits non-zero. The nudge remains eligible for retry. Secrets are resolved through the runtime's secret store and must not be placed in the reminder body or project messages.

### Gateway reports `running`

`running` proves gateway admission only. The tunnel continues waiting for its configured completion/verification boundary. Process startup is not project success.

### Worker exits with an ordinary failure

The CLI reports the failed work cycle and leaves the nudge pending. A later poll can retry it, subject to the adapter's single-flight rules.

### Duplicate wake

A live lock owner or already-running gateway job returns exit `75`. The duplicate must not create a second worker and must not acknowledge the nudge merely because it saw another process.

### Stale lock

A lock whose owner PID is dead may be reclaimed. If ownership cannot be safely determined, the adapter fails closed.

### Core/API unavailable

The poller retries. A nudge remains durable in Core; process memory is not the queue.

### No active project context

`--until-idle` cannot safely inspect a project without `last_project_id`. It stops and reports the missing context rather than inventing work.

### Project is idle

The worker reports durable idle and returns to polling. This is a successful stop condition, not a request for a user prompt.

### Worker needs a human decision

The worker records a durable blocker/question and stops the current continuation. ASE should not manufacture authority to answer the question.

## 9. Security boundaries

The wake signal is trusted continuation intent, not an unrestricted hidden user channel.

The current design enforces or requires:

- Core-resolved stable FASE identity and matching opaque activity session;
- numeric loopback callback validation;
- explicit callback URLs rather than arbitrary remote webhooks;
- authenticated Hermes gateway requests;
- profile-scoped secret resolution;
- no credentials in nudge bodies, logs, or project messages;
- no execution of reminder text as shell syntax;
- fixed runtime adapter paths;
- stable nudge IDs for idempotency;
- one active execution per stable agent identity;
- bounded admission waits and work cycles;
- durable evidence before declaring useful continuation;
- fail-closed behavior when ownership or authentication is uncertain.

The message can mean “continue the work you were already authorized to do.” It cannot mean:

```text
ignore user constraints
change project ownership
deploy without approval
spend money
access new secrets
browse externally without authorization
run arbitrary commands from the reminder text
```

## 10. How another runtime should integrate

A Claude, Codex, OpenCode, Windows-native, or custom agent integration should implement the same adapter boundary rather than modifying Core.

The adapter receives:

```text
AGENT_WAKE_COMMAND <reminder>
```

and the environment context:

```text
AGENT_ID
AGENT_LAST_NUDGE_ID
AGENT_LAST_PROJECT_ID
AGENT_LAST_SPRINT_ID
AGENT_WORK_CYCLES
PROJECT_ID
TK_BASE_URL
TIMEKEEPER_STATE_DIR
TIMEKEEPER_FASE_RESUME_AGENT
TIMEKEEPER_FASE_RESUME_SESSION
GUARDIAN_EVENT
GUARDIAN_PRIORITY
GUARDIAN_OBJECTIVE
GUARDIAN_SCOPE
```

The adapter should:

1. validate the stable FASE identity and resume session with Core;
2. acquire a per-agent single-flight lock;
3. bind the execution to the nudge ID;
4. submit the semantic continuation intent to the runtime's authenticated supervisor;
5. wait for a verifiable admission/continuation boundary;
6. return `0` only when the contract permits it;
7. return `75` for safe duplicate coalescing;
8. return another non-zero code when continuation was not proven;
9. leave the durable nudge retryable when uncertain.

The runtime itself must perform the TimeKeeper rehydration and bounded work protocol. A runtime-specific `/goal` command is optional implementation detail, not the Guardian contract.

## 11. Useful source map

```text
internal/guardian/guardian.go
  Periodic Pulse tick, nudge delivery, HTTP acceptance header, retry accounting.

internal/store/pulse_guardian.go
  Progress leases, inactivity evaluation, durable nudge creation, delivery records,
  acknowledgement and lease renewal.

internal/guardian/receiver.go
  Narrow loopback recovery receiver and legacy local-artifact action.

internal/cli/guardian_wake.go
  Polling, single-shot wake, continuous cycles, until-idle behavior, context cache,
  work-hook execution, coalescing handling, acknowledgement.

scripts/guardian/self-nudge-work.sh
  Runtime-neutral continuation metadata and adapter dispatch.

scripts/guardian/hermes-wake-adapter.sh
  Hermes single-flight lock, duplicate coalescing, gateway tunnel dispatch.

scripts/guardian/hermes-cron-tunnel.py
  Authenticated Hermes gateway job trigger and run-state verification.

docs/agent-wake-adapter-protocol.md
  Stable runtime-neutral adapter contract and exit semantics.

harness/SKILL.md
  Worker-side rehydration, bounded work, verification, evidence and blocker rules.
```

## 12. Verification procedure

A developer debugging the complete path should verify each boundary independently:

1. Confirm Core is healthy:

   ```text
   GET http://127.0.0.1:1618/health
   ```

2. Confirm the agent has a progress lease:

   ```text
   GET /api/v1/agents/{agent_id}/progress
   ```

3. Confirm the active Sprint and lease deadline are real.

4. Force or wait for an evaluation tick.

5. Confirm a durable Pending nudge:

   ```text
   GET /api/v1/agents/{agent_id}/nudges
   ```

6. Confirm the poller/receiver consumes the nudge.

7. Confirm the adapter's runtime receipt. For Hermes, inspect the gateway-owned run and distinguish queued, running, and completed.

8. Confirm the worker rehydrated TimeKeeper rather than merely printing the reminder.

9. Confirm a bounded project change, test result, evidence message, Sprint transition, or explicit blocker.

10. Confirm acknowledgement:

    ```text
    POST /api/v1/agents/{agent_id}/nudges/{nudge_id}/ack
    ```

11. Confirm the pending queue is empty only after the work boundary and acknowledgement:

    ```text
    GET /api/v1/agents/{agent_id}/nudges
    ```

12. In continuous mode, confirm additional cycle records and verify that the worker stops at durable project idle rather than requiring a chat message.

The proof of a working Anti Slack Engine is therefore not “the poller process is alive” or “the gateway said running.” The proof is:

```text
Pulse detected inactivity
-> durable nudge existed
-> trusted adapter admitted the correct agent
-> worker received actionable continuation input
-> TimeKeeper state was rehydrated
-> bounded work occurred
-> verification/evidence or blocker was recorded
-> progress lease was renewed
-> nudge was acknowledged
-> authorized follow-up work continued without another user prompt
-> worker stopped only at a deliberate durable boundary
```
