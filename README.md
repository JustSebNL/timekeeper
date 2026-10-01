# TimeKeeper

Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved. This is proprietary software; see `LICENSE` and `NOTICE`.

**Help agents stay oriented when project work outlasts a single chat.**

A useful project rarely fits in one conversation. Work gets interrupted, agents change, and the next session needs to know what matters and what to do next. TimeKeeper gives people and agents a durable project record they can return to, instead of asking a chat transcript to be the filing cabinet.

## Why it helps

TimeKeeper helps you:

- keep the project goal, next action, and current progress in one place;
- break long work into small, finishable steps;
- leave decisions, blockers, and handoffs for the next session or agent;
- keep a step-by-step, reversible history of edits made in the dashboard's file editor;
- see the same local project record in a browser dashboard or from an agent's tools.

A Sprint is one focused piece of work: a manageable next bite, not “finish the internet before lunch.” Starting, pausing, resuming, or completing it records active-time intervals. It is a planning aid—not a screen monitor, and not a promise that the work is correct. An agent still needs to check its result before saying “done.”

## A gentle first session

Once TimeKeeper is installed and running, open the dashboard at `http://127.0.0.1:1618/`. Choose an existing Project or create one, pick a small next step, and leave a short note when a decision or handoff will help later. That is enough to begin; you do not need to learn the API or every command first.

The repository is proprietary and currently provides a source-based installation, not a downloadable application package. If you have access to a checkout, the [install guide](docs/install.md) has the platform-specific steps. The installer does not add commands to your `PATH`.

For an AI agent using this repository, the intended first-use phrase is:

```text
Use TimeKeeper in the terminal and start using the app.
```

The [agent onboarding Skill](harness/skills/onboarding/SKILL.md) provides a calm, step-by-step introduction. The repository's `AGENTS.md` and [harness contract](harness/SKILL.md) explain the more detailed agent workflow for tools that read and follow repository instructions.

## One agent, or a swarm

TimeKeeper can give several agents a common local view of a Project and its work. It is designed for agents that share access to the same local TimeKeeper service—not for coordinating machines through a hosted cloud workspace. A trusted runtime adapter is still required if an agent must be woken or resumed automatically; TimeKeeper cannot attach itself to an arbitrary chat session.

## What TimeKeeper does not do

- It does not monitor screens or infer that work happened; active time follows the Sprint status you record.
- It does not judge code quality, independently verify evidence, or reconstruct a lost conversation.
- It does not sync projects to the cloud or provide remote, authenticated team hosting.

Your Project data stays on this machine; it is not encrypted automatically. TimeKeeper does not ask you to sign in, so anyone who can run programs as your operating-system user can access the local app. Do not share it over a network, and protect the project folder and any backups.

## Where to go next

- [Getting started](docs/install.md) — install the local app and open the dashboard.
- [Using TimeKeeper with an agent](harness/skills/onboarding/SKILL.md) — the gentle first-use walkthrough.
- [Documentation index](docs/README.md) — choose a guide if you need more detail.
- [Privacy and security](THREAT_REVIEW.md) — what stays local and what the app does not protect.

Detailed command, API, integration, and operator references are currently grouped in the repository's documentation index. A combined documentation and plugin Store portal at `time.mngr.eu.org` is planned, but is not a live documentation link in this checkout. Keep the existing references until the site is published and verified.
