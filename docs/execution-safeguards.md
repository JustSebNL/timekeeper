# Execution safeguards

## Codex CLI supervisor

Launch a new owned Codex CLI process through TimeKeeper:

```powershell
tk supervise --project P-ID --sprint SP-ID --agent worker-unique --model MODEL --state-dir D:/private/tk-runs -- 'Implement the scoped task and report test evidence'
```

Use an Open or On Hold Sprint with a positive estimate. `supervise --help` lists
options, including `--codex` for the executable path and `--stall-timeout` (5m by
default). The project accepts an alias. The model must be explicit. A random suffix is added to the agent label to
avoid overwriting another worker registration; use the checkpoint's exact agent ID
when recording outcomes. Normal Codex
permissions still apply. No process is attached by arbitrary PID.

The supervisor starts `codex exec --json`, forwards its output, registers a
random-token numeric-loopback callback, and runs outside the worker. A Guardian
nudge, expired Sprint, missing completed work item, exhausted task time budget,
API outage, or interruption stops the owned process tree. Windows uses a
kill-on-close job object; Unix uses a process group. Windows containment is
attached immediately after launch, so this is lifecycle supervision, not a
sandbox against deliberately escaping processes.

A callback receipt is not recovery. The supervisor acknowledges a nudge only
after process exit and a persisted Sprint hold. Normal process exit also holds
the Sprint for acceptance verification; a successful process exit never marks
coding work complete. It does not inject prompts into an already-running Codex
turn. Review the checkpoint, justify any permitted extension, then explicitly
launch another supervised run. There are no automatic infinite retries.

Claims are atomic and serialized per Task. At most four supervised launches
(including failed launches) are permitted per Task. Replacing a Sprint does not
reset this count. The task time ceiling is frozen on first launch at twice the
Task estimate plus the first Sprint buffer; the Sprint estimate is the fallback
when the Task has no estimate. Recorded active time across all its Sprints is
subtracted. The supervisor also obeys the smaller Sprint ceiling and polls the
current extended deadline. Exhaustion requires human scope reassessment. These
limits govern this supervisor; direct CLI/API clients remain trusted operators.

Private recovery JSON contains IDs, state, reason, thread ID and cumulative token
counters, never prompt or tool bodies. `hold_pending` means the API hold failed;
`stop_unconfirmed` must be investigated before acknowledging a nudge. A nonzero
`pending_nudge` remains to reconcile. If `usage.turn_seq` exceeds
`usage_persisted_turn`, resend that exact cumulative snapshot with `tk usage record`
after service recovery (same project/session/agent/model/turn/counters). Then apply
the pending hold, verify the tree and acknowledge only a confirmed stopped run.
Do not restart while unresolved recovery files remain. Checkpoints use replacement
writes; disk loss and abrupt host failure are outside the durability guarantee.

## Guardian and extensions

The server's independent Guardian defaults to a five-minute tick. Set
`-pulse-guardian-interval 0` to disable it. The local launcher also accepts an
empty or `0` TIMEKEEPER_PULSE_GUARDIAN_INTERVAL to disable its detector/receiver.
The supervisor's two-second polling and local watchdog remain independent.

Registered agents receive `sprint_overdue` even with a valid progress lease when
active time exceeds estimate, buffer and extensions. Held/terminal Sprints do not
create new deadline nudges. Other expired leases use `agent_unresponsive`.
Nudges and delivery history remain durable. The older standalone receiver only
writes recovery artifacts; use `tk supervise` for Codex process control.

The store permits at most three Sprint extensions, totaling no more than the
original estimate, and rejects extensions of Completed/Cancelled/TimedOut work.
Original estimates and historical extensions are preserved.

## Usage and evidence-linked comparisons

The supervisor captures only reported `turn.completed` usage, accumulating it
into idempotent session snapshots. Codex cached input is subtracted from its inclusive input count before
storage, matching TimeKeeper's separate input/cache buckets and avoiding double counting. An interrupted turn with no usage event has unknown
usage. A model change requires a fresh session. Pricing remains unknown.

Record an acceptance or rejection after inspecting the work:

```json
{
  "session_id": "supervised-ID-FROM-CHECKPOINT",
  "agent_id": "worker-unique",
  "provider": "openai",
  "category": "bugfix",
  "difficulty": "medium",
  "benchmark": "regression-suite-v1",
  "configuration": "model-effort-and-harness-version",
  "result": "accepted",
  "reviewer": "human-or-reviewer-ID",
  "evidence": "commit or diff reference plus reproducible test report",
  "rework_count": 0
}
```

```powershell
tk outcomes record P-ID review.json
tk outcomes P-ID
```

A review requires an existing usage session with a known model and owning Sprint.
It freezes the reviewed turn and counters. Exact retries are idempotent; conflicting
reviews cannot silently overwrite evidence. Comparisons group matching category,
difficulty, benchmark and configuration, then order by reported acceptance rate.
They show sample size, first-pass acceptance, rework, tokens and unreviewed sessions.
Counts represent runs, not whole tasks. Evidence is reviewer-reported, not an
authenticated independent verdict; small samples cannot establish a best coder.
This initial view is CLI/API JSON; there is no new dashboard ranking panel.

API routes: `POST /api/v1/sprints/{sprintID}/supervised-launches` and
`GET|POST /api/v1/projects/{projectID}/coding-outcomes`.

## Verification and rollout

Tests use isolated databases, actual helper child processes, Guardian HTTP delivery,
and simulated API failure. They cover confirmed stop/hold/ack order, unknown usage,
immutable reviews, extension limits and replacement-Sprint launch exhaustion.
They do not spend tokens on an authenticated Codex model run.

Both `tk` and positional `timekeeper` commands use the same client dispatcher.
Build and deliberately roll out the service to activate API/store changes; source
verification alone does not establish that installed binaries contain the changes.

## Friendly URL setup

The additional listener serves the same application handler directly. Its runtime
URLs are exposed at `/api/v1/connection-info`; `doctor` and `open` use that port.
If port 80 cannot bind, the canonical listener also serves the friendly names at
`:1618` (or the configured canonical port). An alternate configured proxy port is
likewise included in generated URLs. No system port reservation is removed.

Both names still need loopback mappings. Run `tk hosts status`; from an Administrator
PowerShell, run `tk hosts add`. The installed `timekeeper.bat hosts add` invokes the
same setup. Status requires both names; add repairs a partial managed block while
preserving unrelated entries. The installer treats hosts setup as best-effort,
so a successful non-admin install does not prove the names were configured.
