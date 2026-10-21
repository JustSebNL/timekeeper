# TimeKeeper CLI and API help

Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

New to TimeKeeper? Start with the [README](README.md) and [install guide](docs/install.md). The complete documentation index is [docs/README.md](docs/README.md).

The `tk` CLI talks to the configured TimeKeeper HTTP API; it does not keep a second local project database. Use `tk help` for the concise command list and `tk api-help` for the route list reported by a running server. `http://timekeeper.local/` is the normal local entry point for people and agents; the CLI uses it by default. Run `tk doctor` if it is unavailable. A second Core server from the same installation is refused with `tk is already running`; multiple CLI commands remain allowed. The installer does not put `tk` on `PATH`; use `.timekeeper/app/tk` on Linux/WSL or `.timekeeper/app/bin/tk.exe` on Windows if needed.

For agent work, load the role-specific workflow in `harness/skills/`: autonomous coder, project planner, bug hunter, bug fixer, deep audit/fixer, gem mining, or code reviewer. The autonomous coder loop continues across bounded Sprints until agreed acceptance criteria are verified or a genuine approval/blocker boundary is reached. The deep audit/fixer traces all first-party functions and integration paths, logs coverage/findings, and repairs verified defects. Use the [Agent Harness Router](docs/agent.md) to select and sequence roles; each role logs decisions/evidence in the source-owning TimeKeeper Project and verifies Sprint state before completion. See [docs/HARNESS.md](docs/HARNESS.md) for first use.

## Common project and Sprint commands

```text
tk doctor
tk list
tk tree <project-id> [--task <T-ID>] [--status S] [--active] [--depth N] [--limit N]
tk export <project-id> [file]
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
tk msg <project-id> search <query...> [--limit N] [--kind K] [--tags T1,T2] [--after DATE] [--before DATE]
tk tc add <project-id-or-alias> <category> <name> <content...> [--subcategory <name>] [--file <path>]
tk tc attach <project-id-or-alias> <gem-id> <name> <content...> [--category C] [--subcategory S] [--file PATH]
tk tc edit <project-id-or-alias> <gem-id> [--name N] [--content TEXT|--file PATH] [--category C] [--subcategory S] [--author A]
tk tc lookup <project-id-or-alias> <query...> [--category C] [--subcategory S] [--pinned] [--include-deleted] [--limit N]
tk tc list <project-id-or-alias> [--category C] [--subcategory S] [--pinned] [--include-deleted] [--limit N]
tk tc show <project-id-or-alias> <gem-id> [--include-deleted]
tk tc pin|unpin <project-id-or-alias> <gem-id>
tk tc delete|restore <project-id-or-alias> <gem-id>
tk tc categories <project-id-or-alias> [--include-deleted]
tk tc revisions <project-id-or-alias> <gem-id>
tk search <query...> [--projects|--tasks|--messages] [--project P-ID] [--limit N] [--kind K] [--tags T1,T2] [--after DATE] [--before DATE]

tk p new <project-name>
tk p edit <project-id> <goal> <description>
tk p status <project-id> <Open|On Hold|Completed|Cancelled>
tk c new <project-id> <category-name> [parent-category-id]
tk c edit <category-id> <goal> <description>
tk c status <category-id> <Open|On Hold|Completed|Cancelled>
tk t new <project-id> <category-id> <task-name> <estimate>
tk t show <task-id> [--json]
tk t edit <task-id> <goal> <description>
tk t status <task-id> <Open|On Hold|Completed|Cancelled>
tk t batch <project-id> <file> [--format format] [--check-only]
tk t export <project-id> <file> [--status S] [--category C]
tk st new <task-id> <subtask-name> <estimate>
tk st show <subtask-id> [--json]
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

Treasure Chest commands persist to TimeKeeper Core's dedicated `project_gems` and `project_gem_revisions` tables in the existing SQLite database. Returned `G-<id>` references identify gems; categories/subcategories are stored fields, and the first gem using a label makes it available automatically. Delete is a reversible soft-delete; `tk tc revisions` reads prior edits. Gem content is limited to 20,000 characters.

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
tk service <install|uninstall|start|stop|restart|status|logs>  # Windows/Linux only; macOS is manual run-only
tk harness list                              List bundled harness guides, Skills, and URLs
tk harness show <listed-item>                Print the complete contents of a listed harness item
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
tk tree P-10000 --task T-10002
tk tree P-10000 --status Open --limit 20
tk t show T-10002
tk st show ST-10003
tk search queue worker --tasks --project P-10000 --limit 10
```

Estimates and durations accept Go duration syntax, for example `90m`, `2h`, or `1h30m`; positive bare integers are interpreted as seconds (for example `3600`). Buffer percentage is a whole number from 0 through 100. Parent IDs are explicit; the CLI does not retain implicit Project context.

`tk doctor` checks readiness, the configured API address, and optional local URLs. When install metadata is available beside the CLI, it also reports the source commit/target and proxy choice. Missing install metadata is informational and does not fail the health check. The `/health` request itself is read-only. Identity-aware CLI builds may also resolve a FASE identity at startup; that is attribution metadata, not authentication or a user login.

## Dashboard

The dashboard is served at `http://timekeeper.local/`, the normal entry point for people and agents. If the hostname or proxy is unavailable, run `tk doctor` and check `tk hosts status` before changing service configuration. The dashboard and CLI use the same local SQLite-backed service. See [Friendly local URLs](docs/friendly-url.md).

The server's five-minute loopback health keep-alive does not write to SQLite, prevent Windows or WSL sleep, or replace OS service recovery. `scripts/service/kick-server.sh` and `.bat` are failure-only recovery hooks; they act only after a failed health check.

`tk pulse` prints Active Sprints whose recorded/current active time exceeds estimate, buffer, and approved extensions. It is read-only: it does not create reminder records, start a timer, send a notification, or contact another service. An empty attention list means no Active Sprint is currently over its recorded plan.

## Pulse Guardian

Pulse Guardian evaluates explicitly registered progress leases and can deliver a versioned attention signal to a registered numeric-loopback callback. It does not by itself know how to resume a particular coding-agent runtime, and TimeKeeper does not kill or restart arbitrary processes. A trusted agent-side adapter owns runtime recovery and must meet the acknowledgement contract. See [Agent wake adapter protocol](docs/agent-wake-adapter-protocol.md) and [Execution safeguards](docs/execution-safeguards.md).

## Import and export

`tk p import <file>` (or `tk import <file>`) imports a full project execution tree (Project, Categories, Tasks, Subtasks, Sprints) from a file or stdin (`-`).

`tk t batch <project-id> <file>` first runs the no-write smoke test, then atomically creates the batch only on PASS. Versioned `timekeeper-execution-batch/v1` JSON/YAML can create nested Tasks, Subtasks, and Sprints; legacy task-only CSV/TSV/XLSX/SQL/archive imports remain supported. Use `--check-only` to validate and print counts without creating objects. Failure output includes object path, field, reason, and repair guidance; a failed check never applies. If the connection drops during apply, inspect the Project tree before retrying because the outcome may be unknown.

Supported format matrix across import and export:

| Extension | Format | Import | Export | Notes |
|---|---|---|---|---|
| `.json` | JSON | ✔ | ✔ | Single JSON execution tree or task array |
| `.yaml` / `.yml` | YAML | ✔ | ✔ | Single YAML execution tree or task list |
| `.xlsx` | Excel | ✔ | ✔ | Multi-sheet workbook (`Project`, `Categories`, `Tasks`, `Subtasks`, `Sprints`) |
| `.csv` | CSV | ✔ | ✔ | Single CSV or archive containing `.csv` files |
| `.tsv` | TSV | ✔ | ✔ | Tab-delimited file or archive containing `.tsv` files |
| `.sql` | SQL | ✔ | ✔ | `INSERT INTO` statements for projects or tasks |
| `.zip` | ZIP Archive | ✔ | ✔ | Archive containing CSV/TSV/JSON/YAML/SQL files |
| `.tar` / `.tar.gz` / `.tgz` / `.gz` | Linux Tarball | ✔ | ✔ | Gzipped tarball containing CSV/TSV/JSON/YAML/SQL files |
| `.rar` | RAR Archive | ✔ | — | Pure Go RAR extraction (`rardecode/v2`); export prints unsupported error |

CSV/TSV/XLSX columns map to Task fields: `name` (required), `category_id`, `description`, `goal`, `priority`, `estimated_duration_seconds`, `estimate` (human-friendly, e.g. `30m`), `auto_close`. Use `-` to read from stdin (defaults to JSON; use `--format` to override).

`tk t export <project-id> <file>` exports tasks to any supported format. Optional filters: `--status Open`, `--category C-10001`. Estimates are written in human-friendly form (e.g. `1h30m`).

`tk export <project-id> <file>` exports the full project execution tree. Without a file argument it prints JSON to stdout. With a file argument, the format is detected from the extension.

## Agentic glue & Plugin Store connectors

TimeKeeper acts as the essential operational glue between AI coding agents working on projects:

- **One-command AI bootstrap**: `tk agent bootstrap` (or `tk init --agent`) synchronizes TimeKeeper harness contracts into `AGENTS.md`, `.cursorrules`, `.windsurfrules`, `CLAUDE.md`, and `.gemini/rules.md`. Any AI tool (Cursor, Windsurf, Claude Code, Copilot, Antigravity, OpenCode, Codex) booting up in the repo automatically discovers TimeKeeper, runs `tk doctor`, and aligns context.
- **Zero-loss context handoff**: `tk context` (or `tk handoff [project-id]`) outputs compact, token-optimized context snapshots containing active Sprint goals, recent trail-of-thought checkpoints (`M-xxx`), active file leases, and Git worktrees.
- **Multi-agent collision guard**: `tk lease claim <task-id|file>`, `tk lease release <task-id|file>`, `tk lease list` manages work boundary leases to prevent concurrent AI agents from stepping on each other's code.
- **Plugin Store connectors**:
  - `tk store search [query]` — search published signed connectors (Jira, Linear, GitHub, Slack, Discord, Obsidian, CI/CD).
  - `tk store install <plugin-id>` — download, verify Ed25519 signature & scan approval evidence, and install connector into `.timekeeper/plugins/`.
  - `tk store list` — view installed connectors and operational status.
  - `tk connect <plugin-id>` — hook an installed connector into the Sprint pipeline and event stream (`tk watch`).
  - `tk connector create <name>` — scaffold a new connector plugin.
  - `tk store pack <dir>` & `tk store publish <pkg.tkp>` — sign and publish connector packages to `TIMEKEEPER_STORE_URL`.

## SQLite backup

Create an overwrite-safe SQLite snapshot with the server backup mode; the destination must not already exist:

```text
timekeeper -db timekeeper.db -backup-to timekeeper-backup.db
```

Project export is not a complete database recovery snapshot. SQLite backups are unencrypted; store both exports and backups accordingly.

## Local SQLite shadow copies

The Core maintains two verified local snapshots (`shadow-a.db` and `shadow-b.db`) using SQLite `VACUUM INTO`; it never copies the live database/WAL directly. Each run performs `PRAGMA integrity_check` on the primary before snapshot execution, checks the staged snapshot before promoting the inactive slot, verifies its SHA-256, and leaves the previous verified slot untouched if any check fails.

Defaults: `.timekeeper/recovery/sqlite-shadow`, every 5 minutes. Configure `-shadow-copy-dir <path>` and `-shadow-copy-interval <duration>`; intervals below 1 minute are rejected, and `0` disables the worker. Prefer a separate available volume for the directory. Each result or skipped/failed health check is written to the service log.

An explicit offline restore is available only after Core is stopped:

```text
timekeeper -db .timekeeper/timekeeper.db -shadow-copy-dir E:/Dev/codebase/Dev/TimeKeeper/.timekeeper/recovery -restore-shadow-slot shadow-a.db -confirm-shadow-restore
```

The command refuses to proceed if the Core lock or listener is occupied, verifies the selected shadow and staged replacement, preserves the current database plus WAL/SHM under `recovery-before-restore-*`, replaces the primary only after those checks, and verifies the installed file. It does not restart Core; start it explicitly after reviewing the result. Restore is never automatic.

## Current boundaries

- Core project tracking runs locally and does not require a cloud account. The server accepts only explicit numeric-loopback listeners and has no network authentication; do not expose it beyond the local user boundary.
- Project, Task, Sprint, notes, messages, events, usage snapshots, local planning drafts, and the alpha bug-report API are available through the CLI/API. Usage is recorded only for sessions that submit snapshots; model cost remains unpriced unless a trusted local price catalogue is configured.
- The `supervise` command owns and bounds processes it launches; it does not attach to arbitrary PIDs or prove that a successful process exit produced correct work. Review and verify before completing a Sprint.
- Local planning supports configured loopback model endpoints. Jev/TypeSafe routing is a separate design and is not enabled by its planning documents.
- Store commands require a separately configured Store. The checked-in default URL is a reserved placeholder; the client does not provide a public live catalogue here. A downloaded package is staged and is not executed by Core.
- There is no hosted remote-server mode, cloud sync, authenticated multi-user deployment, or downloadable TimeKeeper application package in this repository.

For endpoint details, request/response schemas, and security boundaries, see [API.md](API.md), [Agent/API integration](AGENT_INTEGRATION.md), and [THREAT_REVIEW.md](THREAT_REVIEW.md).

tk wt list                                     List Git worktrees and TimeKeeper Sprint/Task status
tk wt new <sprint-id|task-id|branch> [base]    Create an isolated worktree directory and copy shared .env files
tk wt path <sprint-id|branch-name>             Print absolute worktree path for directory navigation
tk wt remove <sprint-id|branch-name> [--force] Remove worktree and safely delete branch
tk wt prune                                    Prune stale Git worktrees

## Git & Repository Integration
- `tk commit [message] [-a|--all]` — Create an attributed commit (e.g. `feat(SP-xxx): message`) and post to Project messages.
- `tk sync` — Sync repo with remote (`git pull --rebase` + `git push`) and log event.
- `tk git status` (or `tk status`) — Show repo status (staged/unstaged files) and active Sprint progress.
