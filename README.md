# TimeKeeper

Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved. This is proprietary software; see `LICENSE` and `NOTICE`.

TimeKeeper is a local work-tracking and memory tool for people and AI agents. It keeps a project moving when work stretches across terminals, days, or conversation windows.

It answers four simple questions:

- What are we trying to finish?
- What is the next concrete piece of work?
- How long should it take?
- What happened, and how do we continue later?

TimeKeeper stores that record locally in SQLite. It does not require a cloud account and it is not a chat service.

## What it is best for

Use TimeKeeper for work that benefits from continuity and accountability:

- software projects and long-running technical tasks;
- work shared between people and coding agents;
- tasks that need honest time estimates;
- handoffs, interruptions, and recovery after a context window ends;
- decisions and discoveries that should not disappear in chat history.

A project is organised as:

```text
Project -> Category -> Task -> optional Subtask -> Sprint
```

A Sprint is one bounded piece of work, usually 15–45 minutes. TimeKeeper records its status, time, notes, evidence, and handoff history.

## Install

Use the one installer for your platform from inside the checkout:

```text
Linux / WSL:  install/install.sh
Windows:     install\\install.bat
```

The installer finds the repository from the installer's own location, builds the local binaries, prepares the dashboard, installs the background service, and verifies the result. It keeps application state inside this checkout's `.timekeeper/` directory.

The installer requires Git and Go to be available. It refuses an unverified checkout rather than building from an unexpected directory or revision. See [`docs/install.md`](docs/install.md) for prerequisites, options, service management, and troubleshooting.

After installation, verify the service:

```text
.timekeeper/app/bin/tk doctor
```

Open the dashboard at:

```text
http://127.0.0.1:1618/
```

Friendly local URLs may also be available as `http://timekeeper.local/` and `http://api.timekeeper.local/` when the hosts-file and proxy setup succeeds.

## Tell an agent to use it

From the TimeKeeper repository, tell the agent exactly this:

```text
Use TimeKeeper in the terminal and start using the app.
```

The repository's [`AGENTS.md`](AGENTS.md) turns that instruction into the workflow automatically. The agent will:

1. read the short harness guide and the relevant Skill;
2. locate the local `tk` command;
3. run `tk doctor`;
4. inspect existing Projects, Tasks, and Sprints;
5. resume suitable work or create a durable record for the next action;
6. record decisions, blockers, evidence, and handoffs;
7. verify work before calling it complete.

The compact first-use guide is [`docs/HARNESS.md`](docs/HARNESS.md). The operational instructions are [`harness/SKILL.md`](harness/SKILL.md), and the project-specific Skills are in [`harness/skills/`](harness/skills/).

An agent can also discover the workflow before it knows the project structure:

```text
timekeeper --help
```

## A human's first commands

```text
tk doctor
tk list
tk tree <project-id>
tk summary <project-id>
tk pulse
```

Use the dashboard when you want to browse the plan visually. Use `tk` when you want a fast terminal workflow. Both use the same local data.

For durable context, record a short note or Trail of Thought checkpoint rather than relying on chat history:

```text
tk msg <project-id> add --kind decision "Use the local adapter first."
tk msg <project-id> checkpoint --author <agent-id> --entry 'evidence|Tests passed.|verification'
```

## Staying on track

TimeKeeper is deliberately honest about unfinished work:

- active Sprints can be checked through `tk pulse`;
- blocked work can be put on hold with a reason;
- failed retrieval attempts remain visible;
- completed work should include real verification evidence;
- context can be recovered from the Project tree, summary, events, and Trail of Thought references.

It does not decide whether an agent's code is good. It keeps the plan and claims visible so a person or another agent can check them.

## Runs locally

The service listens on loopback by default at `127.0.0.1:1618`. Keep it local; do not expose the API to a network without adding proper authentication and transport security.

The local database, dashboard state, logs, and service configuration live under `.timekeeper/`. Do not put credentials, private prompts, or unbounded transcript content into TimeKeeper records.

## More detail

- [First-use harness guide](docs/HARNESS.md)
- [Agent instructions](AGENTS.md)
- [Harness Skill](harness/SKILL.md)
- [Agent/API integration contract](AGENT_INTEGRATION.md)
- [Installation and service management](docs/install.md)
- [Execution safeguards](docs/execution-safeguards.md)
- [API reference](API.md)

## Development

Run the tests from the repository root:

```text
go test ./...
go vet ./...
```

TimeKeeper is a local-first project tool. Its job is simple: keep the work, the clock, and the memory in the same place.
