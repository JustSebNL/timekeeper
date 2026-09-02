# Time Keeper Help

Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

## Implemented CLI

`tk` speaks only to the configured HTTP API. Use `tk --url <api-base-url> <command>` for an explicit endpoint (recommended when a Windows `tk.exe` is run from WSL), or set `TIMEKEEPER_URL` in the binary's native environment. The default is `http://127.0.0.1:1618`; `tk` does not maintain a second local model. Every create command prints public ID, name, and Item Address as tab-separated fields.

```text
tk doctor
tk list
tk tree <project-id>
tk export <project-id> > project-export.json
tk summary <project-id>
tk usage <project-id>
tk usage record <project-id> <session-id> <agent-id> <model> <turn> <input> <output> [cache-write] [cache-read] [messages] [sprint-id]
tk pulse
tk agent progress <agent-id> <lease> [sprint-id] [guardian-url]
tk agent nudges <agent-id>
tk agent history <agent-id>
tk agent ack <agent-id> <nudge-id>
tk events <project-id>
tk note <project-id> <content>
tk notes <project-id>

tk p  new <project-name>
tk p  edit <project-id> <goal> <description>
tk p  status <project-id> <Open|On Hold|Completed|Cancelled>
tk c  new <project-id> <category-name> [parent-category-id]
tk c  edit <category-id> <goal> <description>
tk c  status <category-id> <Open|On Hold|Completed|Cancelled>
tk t  new <project-id> <category-id> <task-name> <estimate>
tk t  edit <task-id> <goal> <description>
tk t  status <task-id> <Open|On Hold|Completed|Cancelled>
tk st new <task-id> <subtask-name> <estimate>
tk st status <subtask-id> <Open|On Hold|Completed|Cancelled>
tk sp new <task|subtask> <owner-id> <sprint-name> <estimate> [buffer-percent]
tk sp next <project-id> # atomically claim the next runnable Sprint
tk sp <start|hold|resume|complete> <sprint-id>
tk sp extend <sprint-id> <duration> <reason>
tk sp extensions <sprint-id>
tk sp entries <sprint-id>

# Optional local planning; pipeline endpoints must be numeric loopback HTTP.
tk llm new <name> <ollama|openai-compatible> <base-url> <model> [system-prompt]
tk plan generate <project-id> <pipeline-id>
tk plan list <project-id>
tk plan apply <project-id> <draft-id>
```

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

Estimates and durations use Go duration syntax, for example `90m`, `2h`, or `1h30m`. Buffer percentage is a whole number from 0 through 100. Parent IDs are explicit; Time Keeper does not retain implicit CLI context, so it cannot quietly operate on the wrong Project after a restart.

`tk doctor` performs a non-mutating `/health` readiness check against the configured endpoint and reports a recovery hint when the API is unavailable.

## Dashboard entrypoint

The canonical browser dashboard is served at `http://127.0.0.1:1618/` from `index.html`. In a source checkout the document is `web/index.html`; the narrow local bootstrap serves the copied document from `.timekeeper/web/index.html`. The dashboard loads the vendored Bootstrap `5.3.8` stylesheet first and Time Keeper's custom stylesheet second. It exposes live project metrics, Pulse/Guardian status, and Recent Activity from immutable Project events. `web/timekeeper.html` is only a compatibility redirect and is not the primary UI document.

The server performs one lightweight loopback `GET /health` keep-alive every five minutes by default. It does not write to SQLite, prevent Windows or WSL sleep, or replace OS service recovery. Pass `-keep-alive-interval 0` to disable it. `scripts/service/kick-server.sh` and `scripts/service/kick-server.bat` are failure-only recovery hooks: they stay silent when `/health` is healthy and restart the service or launcher only after a failed health check.

`tk pulse` prints every Active Sprint whose recorded and current active time has exceeded its estimate, buffer, and approved extensions. It is a read-only local attention check: it does not create reminder records, start a timer, send a notification, or contact another service. `clear\tnext=60s` means there is no such Sprint at this instant.

## Pulse Guardian

`GET /api/v1/pulse` is still the read-only local list of over-plan Active Sprints. It cannot wake a hung agent; it only answers a request that reaches it.

For an actual out-of-band attention path, Time Keeper runs a Guardian by default every 5 minutes. Override the cadence with `-pulse-guardian-interval`, and have the agent renew a material-progress lease:

```text
timekeeper -pulse-guardian-interval 1s

tk agent progress agent:worker-7 45s SP-10004 http://127.0.0.1:19090/pulse
tk agent nudges agent:worker-7
tk agent history agent:worker-7
tk agent ack agent:worker-7 41
```

The registered callback must be a plain numeric-loopback HTTP URL. After a lease has expired, Time Keeper sends one versioned `recover_attention` signal to that independent local Guardian and records whether it explicitly accepted it. The Guardian returns `X-Timekeeper-Pulse-Accepted: v1` after accepting the signal, performs its own deliberate local recovery, and the owning agent acknowledges the nudge. A failed callback remains pending for a later retry; a confirmed callback is not spammed while acknowledgement is pending. Time Keeper itself never kills, restarts, or executes a watched process.

Time Keeper validates serving configuration before opening the database: the dashboard path must be a readable file and `-addr` must use an explicit numeric loopback host (`127.0.0.1` or `::1`). It refuses names, wildcard, and remote interfaces.

## SQLite backup

Create an overwrite-safe, self-contained SQLite snapshot and exit. The destination must not already exist:

```text
timekeeper -db timekeeper.db -backup-to timekeeper-backup.db
```

For a portable Project-only JSON export, use `tk export <project-id> > project-export.json`.

## Availability boundary

Only commands listed in **Implemented CLI** are available. Time Keeper does not currently implement automatic pickup/context recovery, direct process control, Decisions/Edits/Bugs records, task adoption, metrics, external model providers, remote server operation, or a downloadable release installer. Pulse Guardian is limited to explicit material-progress leases and independently-owned numeric-loopback attention callbacks; it is not a general scheduler or process supervisor. A narrow verified-checkout bootstrap is documented in `README.md`.

See `API.md`, `README.md`, `AGENT_INTEGRATION.md`, and `THREAT_REVIEW.md` for the authoritative current contract.
