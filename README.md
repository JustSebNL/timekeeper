# Time Keeper

Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved. This is proprietary software; see `LICENSE` and `NOTICE`.

Time Keeper helps an agent plan work, keep time, and remember what needs attention.

It is a local workspace for the things that usually get lost between conversations: what the agent promised to do, how long it expected the work to take, what is happening now, and what already happened.

## What Time Keeper does

- Plans a Project as Categories, Tasks, Subtasks, and short work blocks called Sprints.
- Gives each Sprint an expected duration and optional buffer, so the plan has a clock instead of a vague "later".
- Tracks whether work is open, active, on hold, complete, or dormant `TimedOut` after four recorded failed retrieval attempts.
- Uses `On Hold` broadly for any real blocker—you run out of road and other items must catch up first, a user decided otherwise, just waiting for input, or any other external dependency—and records the reason without charging active work time when a Sprint has not started.
- Keeps a durable history of changes, notes, time spent, holds, and extensions.
- Surfaces active Sprints that have exceeded their declared plan through a local Pulse.
- Can run an independent local Pulse Guardian that detects an agent which stopped reporting material progress and sends a durable attention-recovery signal to an explicitly registered loopback Guardian.
- Lets an agent or a person inspect the same plan from the dashboard, the `tk` command, or the local API.
- Keeps the plan and history on the local machine. It does not need a cloud account.

Think of it as a project planner, a time scheduler, and a work history for an agent that needs to stay accountable over more than one conversation.

## When work is late

Time Keeper keeps the original time budget, buffer, extensions, current status, and recorded work history together. That means an agent can see why a work block is late instead of guessing from a stale checklist.

## Pulse

Pulse is a local attention check: it highlights Active Sprints only after they exceed their declared plan.

Use the dashboard's Pulse card, `tk pulse`, or `GET /api/v1/pulse` to see the exact work needing attention. This Sprint-overrun view is read-only: it does not create a reminder record, start a background timer, send push notifications, send messages, or contact an external service. An agent can poll it or schedule its own follow-up when that is useful.

## Pulse Guardian

A response from a hung agent is not a nudge — it is just a postcard from a process that may never read it. For an out-of-band recovery path, Time Keeper can run an independent local Pulse Guardian. An agent renews a material-progress lease and explicitly registers a numeric-loopback Guardian callback. When that lease expires, Time Keeper creates a durable recovery nudge and calls the independent Guardian, which may notify, interrupt, restart, or replace its watched agent according to that local integration's own policy.

The split is deliberate: Time Keeper tracks the evidence and delivers only to a registered local Guardian; it does not execute arbitrary commands or silently kill/restart processes. The recovered agent must acknowledge the nudge, so callback acceptance cannot be misrepresented as recovery. See `API.md` and `AGENT_INTEGRATION.md` for the callback contract.

## Where it is going

Over time, Time Keeper should help agents see which models spend tokens without producing useful progress.

That means connecting the time and outcome of a piece of work with the model that handled it. An agent should be able to spot a model that burns a large budget, loops on a task, or creates work that later has to be redone. The point is not to crown a permanent winner. It is to choose the model that is earning its place for this job.

That insight is a direction, not a finished feature. Time Keeper does not yet collect or score token use automatically.

## Install and run

From a clean Time Keeper clone, run:

```text
./install.sh
```

The installer keeps everything it creates inside this clone:

```text
TimeKeeper/
  .timekeeper/
    app/        the installed app and local state
```

It does not put files in your home folder, change your `PATH`, install a service, download code, or start a server without you asking.

Start Time Keeper when you are ready:

```text
./.timekeeper/app/timekeeper
# Pulse Guardian is a required backbone service and runs by default every 5m.
# Override the cadence with TIMEKEEPER_PULSE_GUARDIAN_INTERVAL, or disable it
# explicitly with TIMEKEEPER_PULSE_GUARDIAN_INTERVAL="".
# ./.timekeeper/app/timekeeper -pulse-guardian-interval 1s
# Or when using scripts/run-local.sh, the Guardian is already on by default:
# TIMEKEEPER_PULSE_GUARDIAN_INTERVAL=1s ./scripts/run-local.sh
```

Then open `http://127.0.0.1:1618/` in a browser. In another terminal, these are useful first commands:

```text
./.timekeeper/app/tk doctor
./.timekeeper/app/tk pulse
./.timekeeper/app/tk list
./.timekeeper/app/tk tree <project-id>
```

The server listens only on your own machine. Stop it with `Ctrl+C` when you are done.

## Run as a OS service

TimeKeeper can install itself as a system service so it starts automatically at boot.

```text
# Install and start the service
./.timekeeper/app/tk service install

# Check status
./.timekeeper/app/tk service status

# View logs
./.timekeeper/app/tk service logs

# Stop / restart / remove
./.timekeeper/app/tk service stop
./.timekeeper/app/tk service restart
./.timekeeper/app/tk service uninstall
```

- **Windows**: uses [NSSM](https://nssm.cc) (auto-detected or set `TIMEKEEPER_NSSM`). Installs with `SERVICE_DELAYED_AUTO_START` so it starts shortly after boot.
- **Linux**: uses systemd (user scope, linger-enabled) so it runs without an active login session.

All service artifacts (unit files, logs, NSSM config) live under `.timekeeper/service/` and `.timekeeper/log/`.

## A simple way to use it

1. Create a Project for the outcome you want.
2. Break it into Categories, Tasks, and Subtasks.
3. Give the next piece of work a Sprint with an honest time budget.
4. Start, pause, wait, resume, extend, or finish that Sprint as the work changes. An Open Sprint may be placed on hold before it starts when it is waiting for a person or external dependency; it consumes no active-time budget.
5. Keep retrieval loops bounded: record each failed retrieval attempt, and after the fourth Time Keeper marks the Sprint `TimedOut` while retaining all four reasons.
6. Check Pulse when the agent needs a local list of active work that has run over plan.
7. Check the Project history when the agent needs to recover context or explain what happened.

The dashboard is the easiest place to browse a Project. The `tk` command is useful when an agent or terminal workflow needs the same information.

## What stays local

Time Keeper is for local, single-user work. Its SQLite database, backups, and exported Project history can contain your notes and plans, so keep the clone in a location you trust.

It is not a public service, a team-hosting product, or a remote deployment system. Do not expose its loopback API to a network.

## For developers

Run the local checks with:

```text
GOEXE='/path/to/go'
"$GOEXE" test ./...
"$GOEXE" vet ./...
TIMEKEEPER_GO="$GOEXE" ./scripts/release-preflight.sh
```

`scripts/run-local.sh` is a development shortcut. It builds from the checkout and keeps temporary runtime files in this clone's ignored `.timekeeper/` directory.

The HTTP API is documented in `API.md`. Generic client guidance is in `AGENT_INTEGRATION.md`. The VS Code companion lives in `extensions/vscode/` and uses the same local API.
