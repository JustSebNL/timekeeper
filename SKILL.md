---
name: time-keeper
description: Local HTTP/JSON project hierarchy, sprint timing, notes, and portable export.
---

# Time Keeper Skill

## Scope and boundary

Time Keeper is local-first project tracking with SQLite authority and an HTTP/JSON API. It tracks this implemented hierarchy:

```text
Project → Category → Task → Subtask
                         └→ direct Task Sprint
                 Subtask └→ Subtask Sprint
```

It is not an autonomous-agent runtime. It does not provide pickup/context/adoption shortcuts, heartbeat/pulse escalation, automatic interruption detection, Decisions/Edits/Bugs records, Project completion commands, background recovery, or scheduling. Do not represent those capabilities as present.

The unauthenticated API is deliberately restricted to numeric loopback listeners (`127.0.0.1` or `::1`). Do not expose it on a LAN, public interface, or reverse proxy without a separately designed authentication boundary.

## Start a repository-local instance

From the Time Keeper repository, run:

```bash
TIMEKEEPER_GO='/mnt/c/Program Files/Go/bin/go.exe' ./scripts/run-local.sh
```

The launcher writes ignored runtime state under `.timekeeper/` in the repository, defaults to `127.0.0.1:1618`, and does not install system-wide files or start a service.

Every API mutation requires `Content-Type: application/json`.

## CLI operations

Set `TIMEKEEPER_URL` in the CLI binary's native environment, or use `tk --url <api-base-url> <command>` for an explicit endpoint (recommended when a Windows `tk.exe` is invoked from WSL).

```bash
tk list
tk tree <project-id>
tk summary <project-id>
tk events <project-id>
tk export <project-id> > project-export.json

tk p new <project-name>
tk p edit <project-id> <goal> <description>
tk c new <project-id> <category-name>
tk t new <project-id> <category-id> <task-name> <estimate>
tk st new <task-id> <subtask-name> <estimate>
tk sp new task <task-id> <sprint-name> <estimate> [buffer-percent]
tk sp new subtask <subtask-id> <sprint-name> <estimate> [buffer-percent]
tk sp <start|hold|resume|complete> <sprint-id>
tk note <project-id> <content>
tk notes <project-id>
tk doctor
```

Buffer percentage is a whole number from 0 through 100. Sprint transitions are strictly:

```text
Open → Active → On Hold → Active → Completed
```

## Durable artifacts

- `tk export <project-id>` emits a versioned Project-scoped JSON snapshot to standard output. It is not a database backup.
- `timekeeper -db <source.db> -backup-to <new-backup.db>` creates an overwrite-safe SQLite snapshot with `VACUUM INTO`, then exits. Treat backup files and exports as private local data.
- Project notes are immutable and may be attributed with `X-Agent-ID` through the API.

See `API.md`, `HELP.md`, and `THREAT_REVIEW.md` for the complete current contract and local security boundary.
