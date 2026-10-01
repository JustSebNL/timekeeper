# Jev / TypeSafe integration with TimeKeeper

> Status: Conceptual background; implementation details live in
> [`jev-category-sprint-decision-protocol.md`](jev-category-sprint-decision-protocol.md).
>
> Source reference: https://docs.typesafe.ai/introduction
>
> This document preserves the original architectural exploration. The linked
> protocol is the implementation-facing source for using Jev to decide
> Categories and Sprint destinations. Neither document claims the live Jev
> integration is enabled until the protocol's gates and verification pass.

## Goal

Let an agent identify the project it is working on and describe the work in
plain language, while Jev helps select the appropriate TimeKeeper Category,
Task, Sprint, and follow-up policy.

The intended agent interaction is short:

```bash
tk log --project Example-Project "Fix crash diagnostics upload"
```

When the project is already bound to the current session:

```bash
tk log "Fix crash diagnostics upload"
```

The agent should not need to manually construct a long HTTP request containing
`project_id`, `category_id`, `task_id`, `sprint_id`, provenance, and idempotency
fields for ordinary work. Those details remain important, but the harness or
adapter should supply them.

## Architectural position

The recommended separation is:

```text
Agent / coding framework
        |
        | short intent + project reference
        v
TimeKeeper ergonomic adapter (`tk log`)
        |
        | read candidate project state
        v
ShadowBox Jev adapter
        |
        | typed questions + bounded state
        v
TypeSafe API / Jev
        |
        | Choice / Score / Noul answers
        v
ShadowBox policy composer
        |
        | selected IDs + action + confidence gates
        v
TimeKeeper HTTP API
        |
        v
SQLite, events, snapshots, usage, Trail of Thought
```

Jev is a decision service, not a task database and not the execution engine.
ShadowBox is the boundary for the hosted-model call and its secret. TimeKeeper
remains the authority for project structure, mutations, provenance,
idempotency, file history, and durable state.

No Jev call should write SQLite directly. No ShadowBox worker should invent or
persist a second copy of the TimeKeeper hierarchy.

## Why Jev fits this problem

TypeSafe describes Jev as a System One model: typed questions are evaluated
against state and return structured answers directly. Its primitives are:

- `Choice`: select one item from a fixed set, with probabilities and confidence.
- `Score`: place state on an ordered scale, with probabilities and confidence.
- `Noul`: estimate the probability of a yes/no proposition.

The integration should ask several small questions in one request rather than
ask Jev to produce one broad prose decision. TypeSafe documents that questions
are evaluated in parallel, so the request can cover several independent
judgments without turning the model into a text generator.

Jev does not replace the agent's open-ended reasoning. The agent gathers intent,
explains outcomes, edits code, and performs implementation. Jev handles the
repetitive structured judgments that are currently expressed as manual API
plumbing or brittle rules.

## Proposed request flow

### 1. Resolve the project

The adapter accepts either a stable project ID or a human-facing project name.
Names must resolve to exactly one project. If they do not, the adapter asks for
clarification instead of guessing.

```text
Example-Project -> P-10000
```

The resolved ID is retained as the authority for all subsequent reads and
writes.

### 2. Read bounded candidate state

The adapter retrieves only the state needed for the decision:

- project ID and name;
- category IDs, names, goals, and status;
- open Tasks under those Categories;
- active or recently completed Sprints;
- current session and agent identity;
- concise work intent;
- relevant recent checkpoints or events;
- change/snapshot context when code is being modified.

Do not send credentials, bearer tokens, private prompts, full transcripts, raw
source files, or unbounded logs to Jev. Candidate descriptions should be
compact and redacted.

### 3. Ask atomic questions

A conceptual request could contain questions like these:

```json
{
  "state": {
    "project": {
      "id": "P-10000",
      "name": "Example-Project"
    },
    "intent": "Fix crash diagnostics upload",
    "categories": [
      {"key": "C-1", "name": "Crash diagnostics", "description": "..."},
      {"key": "C-2", "name": "API recovery", "description": "..."},
      {"key": "create_new", "name": "Create a new category", "description": "..."}
    ],
    "tasks": [
      {"key": "T-1", "category_key": "C-1", "name": "Crash reporting", "status": "Open"}
    ]
  },
  "model": "jev-latest",
  "questions": {
    "work_action": {
      "type": "choice",
      "instructions": "What should happen to this work?",
      "criteria": {
        "resume_existing": "Continue an existing matching Task or Sprint",
        "create_task": "Create a new Task under a matching Category",
        "create_category": "Create a new Category because no candidate fits",
        "needs_review": "The evidence is insufficient or conflicting"
      }
    },
    "category": {
      "type": "choice",
      "instructions": "Which Category best owns this work?",
      "criteria": {
        "C-1": "Crash diagnostics and local crash-report collection",
        "C-2": "Server availability, process recovery, and API outages",
        "create_new": "No existing Category is an appropriate owner"
      }
    },
    "needs_code_snapshots": {
      "type": "noul",
      "instructions": "Does this work modify project source code and require before/after snapshots?"
    },
    "severity": {
      "type": "score",
      "instructions": "How consequential is this work if left unresolved?",
      "criteria": [
        "Informational; no user-visible impact",
        "Normal defect or improvement with a workaround",
        "Important degraded behavior affecting users",
        "Blocking or data-loss risk with no acceptable workaround"
      ]
    }
  }
}
```

The exact request shape must follow the released TypeSafe API and SDK. The
example is a design sketch, not a tested payload.

### 4. Compose policy in code

The adapter, not Jev's prose, combines the answers:

```text
work_action = resume_existing
category = C-1
selected task = T-1
needs_code_snapshots = yes
severity = important
```

The composer should preserve the complete answer distributions and explain how
it derived the final action. It should not silently discard a close second
candidate.

### 5. Mutate TimeKeeper through the existing API

The composer sends a normal validated TimeKeeper mutation with:

- project ID;
- selected category/task/subtask/sprint IDs;
- agent ID and session ID;
- ShadowBox run ID;
- Jev model/version;
- decision ID;
- idempotency key;
- selected answer and probability distributions;
- policy threshold and final confidence gate;
- snapshot requirement and change summary.

TimeKeeper reads the selected IDs again and rejects cross-project or stale
references. The decision service is never allowed to bypass those checks.

## Confidence and safety policy

TypeSafe's documentation makes an important distinction: confidence is derived
from the answer's probability distribution and is a useful uncertainty signal,
not a correctness guarantee. The policy must therefore be risk-sensitive and
calibrated with real TimeKeeper examples.

An initial policy could be:

```text
Read-only category suggestion:
  automatically use if Choice confidence >= 0.70

Resume or create ordinary work:
  automatically use if confidence >= 0.85
  otherwise show the proposed destination for confirmation

Create a new Category:
  require confidence >= 0.90 and no existing candidate above 0.50

Close, restore, delete, deploy, or alter durable history:
  never rely on Jev alone; require explicit confirmation and normal API gates

Noul answers:
  use the returned yes-probability; do not pretend Noul has a separate
  confidence field
```

These thresholds are placeholders for experiments, not final product policy.
They must be measured against a labelled set of real routing decisions.

Every Choice should include an escape option such as `create_new`,
`none_of_the_above`, or `needs_review`. Forcing a candidate when none fits is a
routing bug disguised as automation.

## Where code might fit

### TimeKeeper

Potential locations:

- `cmd/tk/main.go` — add an ergonomic `log` command and session project binding.
- `internal/api/` — expose a narrow intent/decision endpoint only if the CLI
  needs a server-side orchestration boundary.
- `internal/model/` — add typed decision/provenance models if decisions become
  first-class records.
- `internal/store/` — persist durable decision records only after the shape is
  proven; do not create a parallel task database.
- `AGENT_INTEGRATION.md` — document the stable low-level contract and the
  ergonomic adapter contract.
- `docs/HARNESS.md` and `harness/SKILL.md` — teach agents the short command and
  the fallback/review behavior.

TimeKeeper should not contain the TypeSafe API key or make an uncontrolled
external call from its core server. Keeping the core provider-neutral makes it
usable offline and preserves the existing authority boundary.

### ShadowBox

Potential locations:

- `internal/jev/` or `internal/decision/` — TypeSafe client, request shaping,
  response validation, redaction, timeout, and retry policy.
- `internal/policy/` — deterministic composition and confidence gates.
- `cmd/shadowbox/` — configuration/diagnostic commands, not secret output.
- ShadowBox configuration — an allowlisted outbound destination and a secret
  reference for `TYPESAFE_API_KEY`.

ShadowBox should record a run reference and sanitized decision summary, but it
should not become the owner of TimeKeeper state.

### Optional separate adapter

If the first implementation should avoid coupling either core project to the
other, create a small adapter service or CLI package outside both cores. It
would call TimeKeeper locally, call TypeSafe through ShadowBox, and expose the
`tk log`-style interface. This is cleaner architecturally but adds another
process and packaging surface, so it is not the first choice unless existing
ShadowBox APIs cannot host the integration.

## Proposed implementation stages

### Stage 0: Contract and fixtures

- Define the short command and response contract.
- Create fixture project trees and labelled routing examples.
- Specify redaction and maximum state sizes.
- Keep all behavior mocked; no TypeSafe credentials required.

### Stage 1: Advisory local adapter

- Add candidate discovery and deterministic formatting.
- Add a mocked Jev client returning typed answers.
- Show the proposed Category/Task/Sprint and probabilities.
- Require confirmation for every mutation.
- Store a compact decision note and idempotency key.

### Stage 2: Real Jev integration in ShadowBox

- Add the TypeSafe SDK or minimal HTTP client.
- Inject `TYPESAFE_API_KEY` only through ShadowBox secrets.
- Add bounded timeouts, retry limits, response-schema validation, and redaction.
- Record model version, latency, usage, and answer distributions.
- Never log the API key or full submitted state.

### Stage 3: Evaluation and calibration

- Replay known TimeKeeper work against Jev.
- Measure category accuracy, abstention rate, false routing, and latency.
- Tune question descriptions and thresholds using held-out examples.
- Keep a deterministic fallback when TypeSafe is unavailable.

### Stage 4: Confidence-gated automation

- Permit automatic low-risk category assignment above calibrated thresholds.
- Keep ambiguous choices in review.
- Require explicit confirmation for destructive or history-changing actions.
- Read back the TimeKeeper mutation and verify the selected destination.

## Failure behavior

The adapter must remain useful when Jev is unavailable or uncertain:

- TypeSafe timeout: do not block all work; fall back to explicit category choice
  or the existing low-level workflow.
- Invalid response: reject the decision and preserve the raw sanitized error.
- Low confidence: request clarification or manual selection.
- Stale candidate IDs: refresh the project tree and retry once with a new
  idempotency key only when the original mutation did not occur.
- TimeKeeper rejection: report the exact structured error and do not claim that
  work was logged.
- ShadowBox policy denial: preserve the decision as an unexecuted proposal.

## Observability and privacy

A decision record should be compact but sufficient to audit the result:

```text
 decision_id
 project_id
 agent_id
 session_id
 shadowbox_run_id
 intent_hash
 selected_action
 selected_category_id
 selected_task_id
 selected_sprint_id
 answers_json (bounded typed answers and probabilities)
 model_version
 policy_version
 threshold_used
 outcome: proposed | applied | reviewed | rejected | failed
 idempotency_key
 created_at
```

Do not retain raw private prompts, credentials, bearer tokens, full transcripts,
or unbounded project dumps. Store a hash or bounded redacted summary of the
intent when the full text is not needed for audit.

## Open questions

1. Does the early-access TypeSafe account expose the API and SDK features needed
   for the proposed request shape?
2. Should the adapter live as a ShadowBox task, a ShadowBox library, or a
   separately packaged bridge?
3. Should decision records be first-class TimeKeeper entities or compact Trail
   of Thought/checkpoint events initially?
4. What candidate count keeps Category/Task choices useful without exceeding
   a bounded state budget?
5. Which historical TimeKeeper decisions can be labelled for calibration?
6. Which actions may be automated, and which always require confirmation?

## Non-goals

This proposal does not:

- replace the agent or LLM used for implementation;
- make Jev responsible for executing shell commands;
- let Jev write TimeKeeper storage directly;
- move TimeKeeper's authority into ShadowBox;
- assume TypeSafe's early-access claims are already validated for this use case;
- remove the raw API for advanced clients;
- silently create or mutate work when confidence is low.
