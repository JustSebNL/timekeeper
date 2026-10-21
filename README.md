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

Once TimeKeeper is installed and running, open the dashboard at `http://timekeeper.local/`. This is the normal entry point for people and agents. If it does not load, run `tk doctor` and check `tk hosts status` to diagnose the friendly hostname or proxy. Choose an existing Project or create one, pick a small next step, and leave a short note when a decision or handoff will help later.

The repository is proprietary and currently provides a source-based installation, not a downloadable application package. If you have access to a checkout, the [install guide](docs/install.md) has the platform-specific steps. The installer does not add commands to your `PATH`.

For an AI agent using this repository, the intended first-use phrase is:

```text
Use TimeKeeper in the terminal and start using the app.
```

The [agent onboarding Skill](harness/skills/onboarding/SKILL.md) provides a calm, step-by-step introduction. The [autonomous coder Skill](harness/skills/autonomous-coder/SKILL.md) defines the bounded continue-until-finished loop for authorized project work. The repository's `AGENTS.md` and [harness contract](harness/SKILL.md) explain the more detailed agent workflow. For role-specific work, the harness includes [project planning](harness/skills/project-planner/SKILL.md), [bug hunting](harness/skills/bug-hunter/SKILL.md), [bug fixing](harness/skills/bug-fixer/SKILL.md), [deep audit and fixing](harness/skills/deep-audit-fixer/SKILL.md), [gem mining](harness/skills/gem-mining/SKILL.md), and [independent code review](harness/skills/code-reviewer/SKILL.md); each records evidence in the owning TimeKeeper Project.

## One agent, or a swarm

TimeKeeper can give several agents a common local view of a Project and its work. It is designed for agents that share access to the same local TimeKeeper service—not for coordinating machines through a hosted cloud workspace. A trusted runtime adapter is still required if an agent must be woken or resumed automatically; TimeKeeper cannot attach itself to an arbitrary chat session.

## What TimeKeeper does not do

- It does not monitor screens or infer that work happened; active time follows the Sprint status you record.
- It does not judge code quality, independently verify evidence, or reconstruct a lost conversation.
- It does not sync projects to the cloud or provide remote, authenticated team hosting.

Your Project data stays on this machine; it is not encrypted automatically. TimeKeeper does not ask you to sign in, so anyone who can run programs as your operating-system user can access the local app. Do not share it over a network, and protect the project folder and any backups.

## Where to go next

- [Getting started](docs/install.md) — install the local app and open the dashboard.
- [Agent onboarding](harness/skills/onboarding/SKILL.md) — the gentle first-use walkthrough.
- [Agent harness router](docs/agent.md) — select the role harness for the work at hand.
- [Documentation index](docs/README.md) — browse the linked guides.
- [Privacy and security](THREAT_REVIEW.md) — what stays local and what the app does not protect.

Detailed command, API, integration, and operator references are currently grouped in the repository's documentation index. A combined documentation and plugin Store portal at `time.mngr.eu.org` is planned, but is not a live documentation link in this checkout. Keep the existing references until the site is published and verified.
