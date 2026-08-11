# Agent and framework integration

Time Keeper is deliberately boring at the boundary: local SQLite behind versioned HTTP JSON. Hermes, OpenClaw, other Claw runtimes, shell agents, and custom orchestrators should use this contract directly rather than receive a runtime-specific fork of the core.

Copyright (c) 2026 Seb. All rights reserved.

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
X-Agent-ID: hermes:default:worker-7
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
7. Create a bounded Sprint below the chosen Task or Subtask; execute it with `start`, `hold`, `resume`, and `complete`.
8. Retrieve immutable time entries rather than re-calculating historical intervals in a framework adapter.
9. Persist returned public IDs and Item Addresses in the framework's own task context.
10. Treat every response as the source of truth; do not derive IDs or reconstruct history from chat.

Example portable HTTP calls:

```bash
curl -sS http://127.0.0.1:1618/health

curl -sS -X POST http://127.0.0.1:1618/api/v1/projects \
  -H 'Content-Type: application/json' \
  -H 'X-Agent-ID: hermes:default:worker-7' \
  -H 'X-Agent-Type: agent' \
  -d '{"name":"HSAM","goal":"Build durable agent memory"}'

curl -sS -X POST http://127.0.0.1:1618/api/v1/projects/P-10000/categories \
  -H 'Content-Type: application/json' \
  -H 'X-Agent-ID: hermes:default:worker-7' \
  -H 'X-Agent-Type: agent' \
  -d '{"name":"Memory","goal":"Own memory subsystems"}'

curl -sS -X POST http://127.0.0.1:1618/api/v1/projects/P-10000/tasks \
  -H 'Content-Type: application/json' \
  -H 'X-Agent-ID: hermes:default:worker-7' \
  -H 'X-Agent-Type: agent' \
  -d '{"category_id":"C-10001","name":"Build recall","estimated_duration_seconds":1800}'
```

## Compatibility rules

- Use endpoint paths, JSON fields, and explicit IDs; do not scrape dashboard HTML.
- Expect unknown future JSON fields and ignore them unless your client needs them.
- Treat a non-2xx response as structured JSON with `error.code` and `error.message`.
- Do not claim runtime recovery, pulses, extensions, cancellation, or token accounting are available until their endpoints are implemented and listed in `API.md`.
- Keep framework adapters outside the Time Keeper core. An adapter may add ergonomic tool schemas, but it must not own a second database or duplicate lifecycle logic.
