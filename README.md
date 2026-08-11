# Time Keeper

Copyright (c) 2026 Seb. All rights reserved. This is proprietary software; see `LICENSE` and `NOTICE`.

Time Keeper is a local-first project execution memory system for humans and agents. SQLite is authoritative; the HTTP API, `tk` CLI, and dashboard use the same data.

## First runnable slice

The current foundation provides:

- server startup validates a readable dashboard path and explicit loopback-only bind address; wildcard/remote interfaces are refused by default
- durable `Project`, `Category`, `Task`, and Task-owned `Subtask` creation with public IDs (`P-10000`, `C-10001`, `T-10002`, `ST-10003`) and hierarchical Item Addresses (`10000.10001.10002.10003`)
- durable, attributed Project notes plus immutable Sprint lifecycle activity, exposed uniformly to HTTP clients, `tk`, and the dashboard
- bounded Sprints owned either directly by a Task or by a Subtask, with an explicit whole-percent planning buffer, legal `Open → Active → On Hold → Active → Completed` lifecycle transitions, and immutable `work`/`hold` time entries
- framework-neutral JSON API: `GET /health`, `GET/POST /api/v1/projects`, a single Project execution-tree read endpoint, hierarchy endpoints for Categories/Tasks/Sprints, Sprint lifecycle endpoints, and `GET /api/v1/sprints/{sprintID}/time-entries`
- a self-contained dashboard at `/` with lazy, inspectable Project → Category → Task → Subtask → Sprint navigation; it can create every core hierarchy item and invoke only the legal Sprint lifecycle actions shown for each current state
- a local overwrite-safe SQLite backup command (`timekeeper -db timekeeper.db -backup-to timekeeper-backup.db`) plus versioned portable Project JSON export via `tk export <project-id>`
- optional local-only LLM planning pipelines for Ollama and OpenAI-compatible loopback servers; models create strict versioned Review drafts only, while a separate user-approved apply action transactionally materializes hierarchy
- API, CLI, and dashboard review/apply paths for local planning drafts; raw model output is inspectable and never treated as authority
- an official local VS Code companion under `extensions/vscode/`, with a hierarchy Explorer, local connection status, Project selection, dashboard deep-link, and legal Sprint lifecycle controls; it uses only the public local HTTP API
- a reviewed local-only threat model and explicit release gates in `THREAT_REVIEW.md`

## Build and run from WSL

The Windows Go toolchain is used from WSL on this machine:

```text
GOEXE='/mnt/c/Program Files/Go/bin/go.exe'
"$GOEXE" test ./...
"$GOEXE" build -o bin/timekeeper.exe ./cmd/server
"$GOEXE" build -o bin/tk.exe ./cmd/tk
./bin/timekeeper.exe -db timekeeper.db -ui web/timekeeper.html
```

Then visit `http://127.0.0.1:1618/` or use:

```text
./bin/tk.exe --url http://127.0.0.1:1618 list
```

### Portable repo-local launch

For a source checkout, `scripts/run-local.sh` builds and runs locally without root access, service managers, package managers, or system-wide installation. Runtime state stays in the ignored `.timekeeper/` directory inside this repository:

```text
TIMEKEEPER_GO='/mnt/c/Program Files/Go/bin/go.exe' ./scripts/run-local.sh
```

### Future user-owned bootstrap contract

A supported installer must discover the checked-out source repository from the invocation directory, but install material and user-owned runtime state under a fixed platform root:

```text
Linux/WSL: ~/.local/share/timekeeper
Windows:   %LOCALAPPDATA%\\TimeKeeper
```

It must not silently install into the invocation directory, a system-wide location, or another hidden root. It must stage safely, refuse unsafe overwrites, require no elevation, and keep source, installed binaries, configuration, and runtime state explicitly distinguishable.

### Source release preflight (not installation)

Before considering a source tree for a release candidate, run its local verification gate:

```text
TIMEKEEPER_GO='/mnt/c/Program Files/Go/bin/go.exe' ./scripts/release-preflight.sh
```

It checks dashboard assets, the disabled-installer sentinel, tests, vet, diff cleanliness, and reproducible local server/CLI builds in a private temporary `.timekeeper/` directory. Passing it does **not** enable `install.sh` or establish deployment, service, signing, or platform-install readiness.

## Agent/framework integration contract

Time Keeper does not assume Hermes, OpenClaw, or any specific Claw runtime. Any tool-capable agent can use ordinary HTTP with its own stable agent metadata:

```text
X-Agent-ID: stable-runtime-worker-id
X-Agent-Name: optional display name
X-Agent-Type: agent | human | system
X-Swarm-ID: optional coordinator/group ID
X-LLM-Provider: supplied only when known
X-LLM-Model: supplied only when known
```

Clients must treat Time Keeper as an external system of record. They should not infer IDs, manufacture historical timers, or depend on a conversation context window for recovery. `tk doctor` performs a non-mutating local readiness check against `/health`; it is the first command to run when a local CLI or agent integration cannot connect. The complete contract is in `API.md`; portable integration guidance for Hermes/OpenClaw/custom runtimes is in `AGENT_INTEGRATION.md`; framework-specific adapters belong outside the core service.

## Publication posture

Do not make this repository public until a security, dependency, documentation, and release review passes. The current evidence and remaining release gates are in `THREAT_REVIEW.md`. If it is ever published before an open-source license is deliberately chosen, the proprietary `LICENSE` remains in force.
