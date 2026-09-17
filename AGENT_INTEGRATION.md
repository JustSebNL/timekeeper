# Agent and framework integration

Time Keeper is deliberately boring at the boundary: local SQLite behind versioned HTTP JSON. Any HTTP/JSON client should use this contract directly rather than receive a runtime-specific fork of the core.

Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

## Connection

Default local base URL:

```text
http://127.0.0.1:1618/api/v1
```

Health is outside the API prefix:

```text
GET http://127.0.0.1:1618/health
```

The server binds to loopback by default. It has no network authentication yet, so do not expose it on a LAN or public interface.

## Identity metadata

Each caller supplies stable identity when it has one. Omit unknown values; Time Keeper must not invent a model or agent identity.

```text
X-Agent-ID: agent:worker-7
X-Agent-Name: optional human-readable name
X-Agent-Type: agent | human | system
X-Swarm-ID: optional coordination group
X-LLM-Provider: supplied only when known
X-LLM-Model: supplied only when known
```

`X-Agent-ID` should survive a process restart when the host framework can provide a durable worker identity. A random per-request ID destroys audit continuity. Tiny paper cut, gigantic forensic headache later.

## Current workflow

1. Check `/health` once before starting work.
2. Create or list a Project.
3. Create Categories below the chosen Project.
4. Create Tasks with the owning `category_id`.
5. Create a Subtask below the Task when execution needs a smaller owner; otherwise create a Sprint directly below the Task.
6. Use `GET /projects/{projectID}/execution-tree` when a client needs the complete hierarchy; direct Task Sprints and Subtask-owned Sprints remain separate collections.
7. Create bounded Sprints below the chosen Task or Subtask. An agent may atomically claim the oldest runnable Sprint with `POST /projects/{projectID}/sprints/claim-next`; otherwise execute an explicit Sprint with `start`, `hold`, `resume`, and `complete`. Claiming is deliberate pickup, not a background worker.
8. Use `GET /pulse` when the agent needs a local, read-only list of Active Sprints over plan.
9. When unattended work needs an out-of-band attention recovery path, renew a material-progress lease at `POST /agents/{agentID}/progress` and register an independently running numeric-loopback Guardian. Time Keeper runs the Guardian by default every 5 minutes; pass `-pulse-guardian-interval` to change the cadence, or an empty interval to disable it, so it evaluates leases outside the watched agent's work loop.
10. A registered Guardian receives a versioned `recover_attention` signal only after the lease expires. It must perform its own deliberate local recovery action, return `X-Timekeeper-Pulse-Accepted: v1` after accepting the signal, and have the recovered owning agent call `POST /agents/{agentID}/nudges/{nudgeID}/ack`.
11. Retrieve immutable time entries rather than re-calculating historical intervals in a framework adapter.
12. Persist returned public IDs and Item Addresses in the framework's own task context.
13. Treat every response as the source of truth; do not derive IDs or reconstruct history from chat.

## Trail of Thought Board

Use the Project Board for short-lived context that deserves long-term survival: reasoning checkpoints, discoveries, decisions, questions, answers, lessons, and source links. Bodies are capped at 1000 Unicode characters. Store only the compact `ref` (for example `M-42`) in the active context, then retrieve the post or complete thread when needed.

```bash
# Record a root thought; response includes M-<id>.
tk msg P-10000 add --kind observation --tags queue,debug "The queue stalls after Sprint promotion."

# Reply without copying the original thought into context.
tk msg P-10000 add --kind answer --reply-to M-42 "The save boundary throws before processSprint."

# Edit current content; history remains immutable.
tk msg P-10000 edit M-42 --kind lesson "Persist state before promoting a Sprint."
tk msg P-10000 checkpoint --author agent-7 --entry 'decision|Persist before dispatch.|queue' --entry 'lesson|Keep only the root M-ref in active context.|handoff'
tk msg P-10000 pin M-42

# Recover context by reference.
tk msg P-10000 show M-42
tk msg P-10000 thread M-42
tk msg P-10000 context M-42 --related-limit 8
tk msg P-10000 revisions M-42
tk msg P-10000 search "processSprint save boundary"
```

The HTTP equivalents are under `/api/v1/projects/{projectID}/messages`. Replies must reference a message in the same Project. Edits use `PATCH` and append a revision; parentage is immutable. The board is not a transcript archive: keep entries atomic, searchable, and small enough to act as durable context anchors.

The `context` command is the preferred recovery path when an agent only has an `M-…` pointer: it returns bounded anchor/thread/related/revision data and reports whether related retrieval used the disposable vector accelerator or FTS fallback.

## Agent usage snapshots

When an adapter can report usage, send cumulative counters at each completed turn. Keep one stable session ID for the conversation and increase `turn_seq` monotonically:

```bash
curl -sS -X POST http://127.0.0.1:1618/api/v1/projects/P-10053/usage-sessions/session-42/snapshots \
  -H 'Content-Type: application/json' \
  -H 'X-Agent-ID: agent:worker-7' \
  -d '{"agent_id":"agent:worker-7","model":"gpt-5","turn_seq":7,"input_tokens":12000,"output_tokens":1800,"cache_creation_tokens":500,"cache_read_tokens":2400,"messages":14}'
```

Counters are cumulative; Time Keeper returns the newly observed delta and treats a repeated session/turn as an idempotent no-op. Send a new session ID if the provider resets its counters. Redact credentials, prompt bodies, private transcript paths, and tool output before sending. Only sessions that submit snapshots are covered; unknown pricing is not guessed. Read a Project's aggregate with `GET /api/v1/projects/{projectID}/usage-summary`.

Example portable HTTP calls:

```bash
curl -sS http://127.0.0.1:1618/health

curl -sS -X POST http://127.0.0.1:1618/api/v1/projects \
  -H 'Content-Type: application/json' \
  -H 'X-Agent-ID: agent:worker-7' \
  -H 'X-Agent-Type: agent' \
  -d '{"name":"Example Project","goal":"Deliver a durable local workflow"}'

curl -sS -X POST http://127.0.0.1:1618/api/v1/projects/P-10000/categories \
  -H 'Content-Type: application/json' \
  -H 'X-Agent-ID: agent:worker-7' \
  -H 'X-Agent-Type: agent' \
  -d '{"name":"Memory","goal":"Own memory subsystems"}'

curl -sS -X POST http://127.0.0.1:1618/api/v1/projects/P-10000/tasks \
  -H 'Content-Type: application/json' \
  -H 'X-Agent-ID: agent:worker-7' \
  -H 'X-Agent-Type: agent' \
  -d '{"category_id":"C-10001","name":"Build recall","estimated_duration_seconds":1800}'
curl -sS -X POST http://127.0.0.1:1618/api/v1/agents/agent%3Aworker-7/progress \
  -H 'Content-Type: application/json' \
  -d '{"active_sprint_id":"SP-10004","lease_duration_seconds":45,"guardian_url":"http://127.0.0.1:19090/pulse"}'
```

The Guardian callback is a deliberately narrow local boundary: numeric loopback `http` URL, no credentials/query/fragment, 3-second delivery timeout, redirects refused, and an explicit `X-Timekeeper-Pulse-Accepted: v1` acknowledgement. Time Keeper stores the nudge and delivery evidence, but never executes recovery commands itself. The callback process — outside the watched agent — decides whether a nudge means display, interrupt, restart, or replacement.

- Use endpoint paths, JSON fields, and explicit IDs; do not scrape dashboard HTML.
- Expect unknown future JSON fields and ignore them unless your client needs them.
- Treat a non-2xx response as structured JSON with `error.code` and `error.message`.
- Do not claim direct process control, token accounting, remote delivery, or recovery actions beyond a registered local Pulse Guardian unless the adapter has actually implemented and tested them.
- Keep framework adapters outside the Time Keeper core. An adapter may add ergonomic tool schemas, but it must not own a second database or duplicate lifecycle logic.

## Execution safeguard update

See [Execution safeguards](docs/execution-safeguards.md) for bounded extensions, sprint deadline nudges independent of progress leases, legacy nudge migration, immutable model attribution, and the distinction between callback receipt and actual agent recovery.
