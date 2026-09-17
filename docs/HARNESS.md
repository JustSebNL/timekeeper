# TimeKeeper first use: agent quick start

Use this when an agent encounters TimeKeeper for the first time. The goal is not to memorize the API; the goal is to enter the project workflow safely through the harness Skill and the relevant project-local Skill.

## 1. Start with the harness Skill

Read [`harness/SKILL.md`](../harness/SKILL.md) first. It is the operational
contract for using TimeKeeper and describes the one bounded recovery action if
the service is unavailable. The project-local Skills are listed under
`harness/skills/`; load the one relevant to the next action.

Check the runtime:

```bash
tk doctor
```

If it fails, follow the recovery procedure in `harness/SKILL.md`, then run
`tk doctor` again. Do not start a second server, create a second database, or
bypass the durable record by tracking work only in chat.

## 2. Load the relevant Skill

Before planning or changing a project, inspect the project-local Skills:

```text
harness/skills/project-landscape/SKILL.md
harness/skills/backlog-sync/SKILL.md
harness/skills/codebase-audit/SKILL.md
harness/skills/timeboxing/SKILL.md
harness/skills/troubleshooting/SKILL.md
```

Load only the Skill relevant to the next action. The harness contract is [`harness/SKILL.md`](../harness/SKILL.md); its machine-readable manifest is [`harness/manifest.yaml`](../harness/manifest.yaml).

## 3. Discover or create the work record

```bash
tk doctor
tk list
tk tree <project-id>
tk summary <project-id>
```

Use returned IDs exactly. If no suitable project exists, establish:

```text
Project -> Category -> Task -> optional Subtask -> Sprint
```

A Sprint is one bounded action, preferably 15–45 minutes and small enough to produce evidence. Do not create a vague “finish everything” Sprint.

## 4. Work, verify, and leave a handoff

Execute the next concrete Sprint. Record important decisions, discoveries, blockers, and handoffs as a note or Trail of Thought post. Use `On Hold` for a real external blocker rather than silently burning time.

Before completing the Sprint, run a real check: test, build, reproduction, file read-back, or another explicit verification. Then record the evidence and read the state back:

```bash
tk pulse
tk events <project-id>
tk msg <project-id> checkpoint --author <agent-id> \
  --entry 'evidence|Tests/build/reproduction result.|verification' \
  --entry 'next|Next concrete action.|handoff'
```

After interruption, repeat `tk doctor`, `tk tree`, `tk summary`, and read the latest events/checkpoint before continuing. Do not infer state from an old conversation.

## Rules worth remembering

- TimeKeeper is the durable project record; chat context is temporary.
- The harness is the first operational step; the relevant Skill is the first planning step.
- Evidence beats “done” in model text.
- Never store credentials, private prompt bodies, or unbounded logs in TimeKeeper.
- Use [`AGENT_INTEGRATION.md`](../AGENT_INTEGRATION.md) for the complete CLI/API, Trail of Thought, Guardian, and usage-snapshot contracts.

The shortest reliable path is:

```text
harness -> Skill -> discover -> bounded Sprint -> verify -> record -> continue
```
