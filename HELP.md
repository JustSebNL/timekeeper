# TimeKeeper CLI and API help

Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

New to TimeKeeper? Start with the [README](README.md) and [install guide](docs/install.md). The complete documentation index is [docs/README.md](docs/README.md).

The `tk` CLI talks to the configured TimeKeeper HTTP API; it does not keep a second local project database. Use `tk help` for the concise command list and `tk api-help` for the route list reported by a running server. The installer does not put `tk` on `PATH`; use `.timekeeper/app/tk` on Linux/WSL or `.timekeeper/app/bin/tk.exe` on Windows if needed.

## Common project and Sprint commands

```text
tk doctor
tk list
tk tree <project-id>
tk export <project-id> > project-export.json
tk summary <project-id>
tk usage <project-id>
tk usage record <project-id> <session-id> <agent-id> <model> <turn> <input> <output> [cache-write] [cache-read] [messages] [sprint-id]
tk usage ingest <project-id> <envelope.json>
tk pulse
tk events <project-id>
tk note <project-id> <content>
tk notes <project-id>
tk msg <project-id> add [--kind <kind>] [--reply-to M-1] <body>
tk msg <project-id> checkpoint --entry kind|body|tags [--entry kind|body|tags]

tk p new <project-name>
tk p edit <project-id> <goal> <description>
tk p status <project-id> <Open|On Hold|Completed|Cancelled>
tk c new <project-id> <category-name> [parent-category-id]
tk c edit <category-id> <goal> <description>
tk c status <category-id> <Open|On Hold|Completed|Cancelled>
tk t new <project-id> <category-id> <task-name> <estimate>
tk t edit <task-id> <goal> <description>
tk t status <task-id> <Open|On Hold|Completed|Cancelled>
tk st new <task-id> <subtask-name> <estimate>
tk st status <subtask-id> <Open|On Hold|Completed|Cancelled>
tk sp new <task|subtask> <owner-id> <sprint-name> <estimate> [buffer-percent]
tk sp next <project-id>
tk sp <start|hold|resume|complete> <sprint-id>
tk sp extend <sprint-id> <duration> <reason>
tk sp extensions <sprint-id>
tk sp entries <sprint-id>

# Optional local planning; configured model endpoints must be numeric loopback HTTP.
tk llm new <name> <ollama|openai-compatible> <base-url> <model> [system-prompt]
tk plan generate <project-id> <pipeline-id>
tk plan list <project-id>
tk plan apply <project-id> <draft-id>
```

Other implemented command groups include agent identity/activity, Guardian wake, bounded Codex supervision, reviewed coding outcomes, the optional Store client, host-file management, service management, browser opening, aliases, notifications, version, and API help. Common agent/runtime forms are:

```text
tk agent identity
tk agent state [agent-id]
tk agent states
tk agent work <sprint-id> <lease>
tk agent wait <duration> <reason>
tk agent idle [reason]
tk agent stop [reason]
tk agent progress <id> <lease> [sprint-id] [guardian-url]
tk agent nudges <id>
tk agent history <id>
tk agent ack <id> <nudge-id>
tk agents list
tk agents broadcast

tk guardian wake --agent <id> --project <project-id> --adapter <trusted-executable> [--poll]
tk init --agent <id> [--project <project-id>] [--lease <duration>]
tk supervise --project P-ID --sprint SP-ID --agent ID --model MODEL --state-dir DIR -- PROMPT

tk open [--ip|--api]
tk hosts <add|remove|status>
tk service <install|uninstall|start|stop|restart|status|logs>
tk api-help
```

`tk init` is optional receiver/lease setup, not required for ordinary Project tracking. `guardian wake` needs an explicit adapter or a trusted, explicitly configured hook. Run `tk help` for the commands available in the installed CLI; options can vary by build.

Examples:

```text
tk p new "Project"
tk p edit P-10000 "Project goal" "Durable Project description"
tk c new P-10000 "Backend"
tk t new P-10000 C-10001 "API persistence" 4h
tk st new T-10002 "Migration" 30m
tk sp new subtask ST-10003 "Implement migration" 25m 20
tk sp start SP-10004
tk sp extend SP-10004 10m "Migration uncovered an additional compatibility case."
tk tree P-10000
```

Estimates and durations use Go duration syntax, for example `90m`, `2h`, or `1h30m`. Buffer percentage is a whole number from 0 through 100. Parent IDs are explicit; the CLI does not retain implicit Project context.

`tk doctor` checks readiness, the configured API address, and optional local URLs. When install metadata is available beside the CLI, it also reports the source commit/target and proxy choice. Missing install metadata is informational and does not fail the health check. The `/health` request itself is read-only. Identity-aware CLI builds may also resolve a FASE identity at startup; that is attribution metadata, not authentication or a user login.

## Dashboard

The dashboard is served at `http://127.0.0.1:1618/` by the local TimeKeeper service. It and the CLI use the same local SQLite-backed service. Friendly hostnames may also work when their hosts-file entries and proxy listener are available; see [Friendly local URLs](docs/friendly-url.md).

The server's five-minute loopback health keep-alive does not write to SQLite, prevent Windows or WSL sleep, or replace OS service recovery. `scripts/service/kick-server.sh` and `.bat` are failure-only recovery hooks; they act only after a failed health check.

`tk pulse` prints Active Sprints whose recorded/current active time exceeds estimate, buffer, and approved extensions. It is read-only: it does not create reminder records, start a timer, send a notification, or contact another service. An empty attention list means no Active Sprint is currently over its recorded plan.

## Pulse Guardian

Pulse Guardian evaluates explicitly registered progress leases and can deliver a versioned attention signal to a registered numeric-loopback callback. It does not by itself know how to resume a particular coding-agent runtime, and TimeKeeper does not kill or restart arbitrary processes. A trusted agent-side adapter owns runtime recovery and must meet the acknowledgement contract. See [Agent wake adapter protocol](docs/agent-wake-adapter-protocol.md) and [Execution safeguards](docs/execution-safeguards.md).

## SQLite backup and Project export

Create an overwrite-safe SQLite snapshot with the server backup mode; the destination must not already exist:

```text
timekeeper -db timekeeper.db -backup-to timekeeper-backup.db
```

For a portable Project-only JSON export, use `tk export <project-id> > project-export.json`. Project export is not a complete database recovery snapshot. SQLite backups are unencrypted; store both exports and backups accordingly.

## Current boundaries

- Core project tracking runs locally and does not require a cloud account. The server accepts only explicit numeric-loopback listeners and has no network authentication; do not expose it beyond the local user boundary.
- Project, Task, Sprint, notes, messages, events, usage snapshots, local planning drafts, and the alpha bug-report API are available through the CLI/API. Usage is recorded only for sessions that submit snapshots; model cost remains unpriced unless a trusted local price catalogue is configured.
- The `supervise` command owns and bounds processes it launches; it does not attach to arbitrary PIDs or prove that a successful process exit produced correct work. Review and verify before completing a Sprint.
- Local planning supports configured loopback model endpoints. Jev/TypeSafe routing is a separate design and is not enabled by its planning documents.
- Store commands require a separately configured Store. The checked-in default URL is a reserved placeholder; the client does not provide a public live catalogue here. A downloaded package is staged and is not executed by Core.
- There is no hosted remote-server mode, cloud sync, authenticated multi-user deployment, or downloadable TimeKeeper application package in this repository.

For endpoint details, request/response schemas, and security boundaries, see [API.md](API.md), [Agent/API integration](AGENT_INTEGRATION.md), and [THREAT_REVIEW.md](THREAT_REVIEW.md).
