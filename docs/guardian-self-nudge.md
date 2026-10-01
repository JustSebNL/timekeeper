# Guardian Self-Nudge: From Chatbot to Worker

**Created:** 2026-09-23
**Status:** Design + Implementation

---

## Problem

The agent idles between user messages. The Pulse Guardian already detects expired agent leases and sends nudges to registered callback URLs. But there's no mechanism for the agent to **use** that nudge to start working — it just acknowledges and goes back to sleep.

## Goal

When the guardian fires a nudge, the agent should:
1. Wake up
2. Check for pending work (pulse, events, sprints)
3. Do the next concrete action
4. Repeat until no work remains
5. Then acknowledge the nudge and go back to idle

## Architecture

```
TimeKeeper Server (port 1618)
  └── Pulse Guardian (internal/guardian) — the SENDER
        ├── Watches agent_pulse_progress for expired leases
        ├── Creates pulse_nudges (kind: agent_unresponsive)
        └── POSTs nudge to guardian_url (registered callback)
              │
              ▼
        Self-Nudge Receiver (cmd/tk-guardian-self, port 1619) — the LISTENER
          ├── HTTP server on 127.0.0.1:1619 (loopback only)
          ├── Receives nudge signal from guardian
          ├── Triggers work cycle:
          │     1. tk pulse → check for overdue sprints
          │     2. tk sp next → claim next sprint
          │     3. Do work (build, test, verify)
          │     4. tk msg add --kind observation → record evidence
          │     5. tk sp complete → close sprint
          │     6. Repeat until no work
          ├── Acknowledges nudge back to TimeKeeper API
          └── Goes back to idle
```

**Key distinction:** The pulse engine (inside TimeKeeper) SENDS nudges.
The receiver (separate process) LISTENS for them. The receiver is not a
pulse engine — it is a work-cycle trigger.

## Components

### 1. Self-Nudge Receiver (`cmd/tk-guardian-self/`)

A lightweight HTTP server that:
- Binds to `127.0.0.1:1619` (loopback only, never exposed)
- Accepts POST `/pulse` with the guardian signal
- On nudge: runs the work cycle, then acks
- On GET `/healthz`: returns "ok" for liveness probes

### 2. Work Cycle Script (`scripts/guardian/self-nudge-work.sh`)

A shell script that:
1. Runs `tk pulse` to check for overdue sprints
2. If overdue sprints exist, claims the oldest via `tk sp next`
3. Runs a build/test cycle
4. Records evidence via `tk msg add --kind observation`
5. Completes the sprint via `tk sp complete`
6. Repeats until `tk pulse` shows clear

### 3. Registration (`tk guardian self-register`)

Registers the agent with the guardian:
- `POST /api/v1/agents/{id}/progress` with a lease
- Sets `guardian_url` to `http://127.0.0.1:1619/pulse`
- The guardian will fire a nudge when the lease expires

### 4. CLI (`tk guardian self-nudge`)

Starts the self-nudge loop:
- Registers the agent (if not already registered)
- Starts the receiver
- On nudge: runs work cycle, acks, waits for next nudge

## Security

- Receiver binds to loopback only (`127.0.0.1`)
- No remote execution — only local CLI commands
- Nudge payload is validated against the expected format
- Work cycle runs with the same permissions as the CLI user

## Files

- `cmd/tk-guardian-self/main.go` — receiver + work cycle
- `scripts/guardian/self-nudge-work.sh` — work cycle script
- `internal/guardian/self_nudge.go` — registration helper
- `docs/guardian-self-nudge.md` — this document

## Usage

```bash
# Start the self-nudge loop (registers agent + starts receiver)
tk guardian self-nudge --agent-id example-agent --lease 30m

# The agent will now:
# 1. Register with the guardian
# 2. Wait for nudges
# 3. On nudge: do work, ack, wait for next nudge
# 4. On Ctrl+C: unregister and exit
```

## Hermes Agent wake bridge

The full operator and agent usage guide is [`anti-slack-engine.md`](anti-slack-engine.md). It documents the friendly Anti Slack Engine name, the Pulse Guardian protocol, cross-platform wake CLI, custom adapter contract, callback acceptance header, lease discipline, ntfy/poll fallback, and recovery troubleshooting.

The Pulse Guardian callback is not the same thing as a running Hermes
conversation. A registered callback with no listener is only a dead address,
and a callback that acknowledges a failed work hook loses the recovery signal.
The default work hook therefore fails closed and never claims or completes a
Sprint by itself.

For Hermes, install a stable monitor under `~/.hermes/scripts/` and attach it
to a scheduled agent job. The monitor reads `tk pulse`, removes changing
runtime durations, and emits only the stable overdue Sprint set. Hermes runs
the reminder prompt only when that set changes:

```bash
cp scripts/guardian/timekeeper-pulse-monitor.sh ~/.hermes/scripts/
hermes cron create --name 'TimeKeeper idle recovery' --deliver local \
  --monitor-script timekeeper-pulse-monitor.sh \
  --workdir /path/to/TimeKeeper --skill timekeeper-development 'every 5m' \
  'You are idling. Continue the remaining TimeKeeper work. Inspect the real repository state, use TimeKeeper as the authority, and do not complete a Sprint without actual work and verification.'
```

The reminder is intentionally explicit: `You're idling — please continue
remaining work.` This is the activation boundary. TimeKeeper detects and
reports attention; Hermes supplies the process that can start a new agent run.
`--deliver local` keeps the wake internal. Use a gateway delivery target only
when an external chat notification is wanted.

## Future Work


- Work cycle as a pluggable script (not hardcoded)
- Multiple work cycles (build, test, review)
- Integration with Shadowbox for sandboxed work
- Dashboard showing self-nudge activity

---

*This document lives in `docs/` as a design reference.*
