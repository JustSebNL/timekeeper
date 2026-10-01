# Jev Decision Protocol for TimeKeeper Categories and Sprints

Status: implementation blueprint; Jev/TypeSafe execution is not yet enabled in the current product
Tracking: P-10053/T-10468 — deliver this as an optional Store/plugin integration, not a mandatory Core dependency.
Owner boundary: TimeKeeper owns durable work state; ShadowBox owns the model-call boundary; Jev returns typed decisions; deterministic policy code decides whether anything may be applied

This document specifies everything required to use Jev as a decision maker for routing work into the correct TimeKeeper Category and for deciding whether to resume or create a bounded Sprint.

The retrieval-oriented Jev knowledge base is in [`Jev-kb/`](Jev-kb/). It splits the domain into Project selection, Category assignment, Task detection, Sprint detection/creation, queue lifecycle, policy, examples, output schema, runtime instructions, and maintenance rules.

## ASE/FASE constraint

Jev decisions run in the context of the resolved FASE actor, but FASE-ID is
attribution rather than authentication. A Jev answer cannot acquire or bypass
an execution claim. Before applying a recommendation, the deterministic policy
composer must reread the candidate tree and use the normal TimeKeeper mutation
path with the current actor. A competing claimant returns structured
`409 item_already_claimed` and routes to review; it must not be retried as
another identity. Guardian wake/ACK restores the bound actor context but is not
progress. Notes, evidence, and child work remain collaborative contributions.

It is intentionally explicit about what is implemented today and what remains to build. The existing TimeKeeper planning-draft feature can already generate and apply a validated plan through an LLM pipeline, but that is not the same as a confidence-gated Jev decision service. Do not describe Jev routing as live until the gates and verification in this document exist.

## 1. The outcome we want

An agent should be able to say:

```text
Fix crash diagnostics upload
```

or use the ergonomic target command:

```bash
tk log --project Example-Project "Fix crash diagnostics upload"
```

The agent should not have to manually discover and pass a chain of IDs merely to record ordinary work:

```text
project -> category -> task -> optional subtask -> sprint
```

Jev should help answer two related questions:

1. Which existing TimeKeeper Category owns this intent, or is a new Category genuinely required?
2. Given that Category and its Tasks, should the work resume an existing runnable Sprint, create a new Task/Sprint, or stop for review?

Jev does not create rows, start Sprints, edit source files, or complete work. Its answer is a typed proposal. Deterministic policy code validates the proposal, and TimeKeeper remains the only authority allowed to mutate its hierarchy.

The intended end state is:

```text
agent intent
  -> resolve project deterministically
  -> read authoritative candidate tree
  -> bounded Jev category decision
  -> deterministic candidate narrowing
  -> bounded Jev task/Sprint decision
  -> confidence/risk policy
  -> proposal or safe mutation request
  -> TimeKeeper validates IDs, parentage, lifecycle, and idempotency
  -> read back exact applied state
  -> record decision and outcome
```

## 2. Non-negotiable authority boundary

The system has four distinct responsibilities:

```text
Agent / coding framework
  Understands the work, supplies intent, performs implementation, explains results.

TimeKeeper adapter / policy composer
  Discovers candidates, asks typed questions, combines answers, applies thresholds,
  builds an idempotent validated mutation request, and records provenance.

ShadowBox Jev adapter
  Holds the TypeSafe credential, submits bounded state/questions, validates the
  response schema, applies timeout/retry/redaction rules, and returns typed answers.

TimeKeeper Core / Store
  Owns Projects, Categories, Tasks, Subtasks, Sprints, statuses, parentage,
  events, idempotency, evidence, and durable decision outcomes.
```

Jev must never:

- write SQLite directly;
- call TimeKeeper mutation routes directly without the policy composer;
- invent a Category, Task, Sprint, or identifier outside the candidate protocol;
- start or complete a Sprint;
- execute shell commands;
- access source files or unrestricted transcripts;
- receive credentials, bearer tokens, or private connection strings;
- override a parent status, Sprint lifecycle rule, approval gate, or project owner;
- silently turn a low-confidence answer into a durable mutation.

ShadowBox must not become a second TimeKeeper database. It may retain a run reference and a sanitized decision envelope, but TimeKeeper remains the authority for the resulting hierarchy.

## 3. Current TimeKeeper hierarchy and lifecycle

The durable hierarchy is:

```text
Project (P-)
  -> Category (C-)
      -> Task (T-)
          -> optional Subtask (ST-)
              -> Sprint (SP-)
```

Every Sprint belongs to one Project through its parent Task or Subtask. A Sprint never floats independently.

### 3.1 Category

A Category is a Project-owned work area. It has an ID, name, goal/context, status, and parent Project. A Jev category decision selects an existing `C-` candidate or explicitly proposes `create_new`.

Category selection must be scoped to exactly one resolved Project. A Category from another Project is invalid even if its name is a perfect semantic match.

### 3.2 Task

A Task belongs to a Category. Jev does not route directly from Project to Sprint. After category selection, policy narrows the Task candidates to Tasks under that Category. The second decision may select an existing Task or propose a new Task.

### 3.3 Subtask

A Subtask is optional. It is useful when an existing Task needs a smaller owner. The second-pass candidate state may include existing Subtasks, but the first implementation may omit automatic Subtask creation and route that case to review.

### 3.4 Sprint

A Sprint is a bounded execution unit under a Task or Subtask. The current queue contract enforces:

- one to four direct Sprints per Task or Subtask;
- parent Project, Category, Task, and optional Subtask must be open when creating a Sprint;
- `Open -> Active` through `start` or `claim-next`;
- `Active -> On Hold` through `hold` with a reason;
- `On Hold -> Active` through `resume`;
- `Active -> Completed` through `complete` with evidence;
- `Active -> TimedOut` after the configured retrieval-attempt limit;
- explicit cancellation is terminal;
- an `Open` Sprint cannot be completed without first becoming `Active`;
- an `Open` Sprint under a non-open parent is visible but not runnable;
- held and terminal Sprints are not treated as runnable work;
- completed, timed-out, and cancelled history is not deleted.

The current relevant API routes are:

```text
GET  /api/v1/projects/{projectID}/execution-tree
GET  /api/v1/projects/{projectID}/operational-summary
GET  /api/v1/projects/{projectID}/attention
POST /api/v1/projects/{projectID}/sprints/claim-next

GET  /api/v1/projects/{projectID}/categories
POST /api/v1/projects/{projectID}/categories

GET  /api/v1/projects/{projectID}/tasks
POST /api/v1/projects/{projectID}/tasks

GET  /api/v1/tasks/{taskID}/sprints
POST /api/v1/tasks/{taskID}/sprints
GET  /api/v1/subtasks/{subtaskID}/sprints
POST /api/v1/subtasks/{subtaskID}/sprints

POST /api/v1/sprints/{sprintID}/start
POST /api/v1/sprints/{sprintID}/hold
POST /api/v1/sprints/{sprintID}/resume
POST /api/v1/sprints/{sprintID}/complete
POST /api/v1/sprints/{sprintID}/cancel
```

The exact route table in `internal/api/server.go` and validation in `internal/store/` are authoritative. The adapter must not infer a route from the noun hierarchy; it must use the actual route contract.

## 4. Current implementation versus target

| Capability | Current state | Target for Jev routing |
|---|---|---|
| TimeKeeper hierarchy and Sprint lifecycle | Implemented in Store/API | Reuse unchanged |
| Execution tree discovery | Implemented | Bounded candidate extractor |
| Planning draft generation | Implemented with configured LLM pipeline | Keep separate from Jev typed routing |
| Jev/TypeSafe client | Not implemented as a live integration | ShadowBox-only typed decision client |
| Category decision | Manual/agent/API workflow | Advisory first, then confidence-gated |
| Task/Sprint decision | Manual/agent/API workflow | Two-pass Jev decision after category narrowing |
| Decision record | Use Project events/messages initially | Dedicated record may follow after contract stabilizes |
| Automatic Category mutation | Not enabled | Only after calibration and threshold gate |
| Automatic Sprint creation | Not enabled | High threshold, bounded estimate, idempotent API mutation |
| Human confirmation | Required for every Stage 1 mutation | Remains mandatory for new Category and high-risk actions |
| Fallback when Jev unavailable | Manual low-level API/CLI | Must remain deterministic and usable offline |

The words “decision maker” mean Jev makes a typed recommendation. They do not mean Jev receives authority to mutate TimeKeeper.

## 5. End-to-end decision flow

### Step 0: Receive an intent

The caller supplies:

```json
{
  "project": "Example-Project",
  "intent": "Fix crash diagnostics upload",
  "agent_id": "agent:worker-7",
  "session_id": "session-42",
  "source": "cli",
  "provenance": "agent-request"
}
```

The intent is bounded by input-size limits. It may include a concise context summary, but it must not be an unbounded transcript or raw repository dump.

The ergonomic CLI should accept a project name or stable ID. If a name maps to zero or multiple Projects, the adapter must stop for clarification. It must never choose the first fuzzy match.

### Step 1: Resolve the Project deterministically

Resolve the project name to exactly one canonical `P-` ID through TimeKeeper. Retain both the supplied display name and resolved ID in the decision envelope.

```text
Example-Project -> P-10000
```

All later candidate reads and writes use `P-10000`. The policy composer must reject any selected Category, Task, Subtask, or Sprint that does not belong to this Project.

### Step 2: Read the authoritative tree

Fetch the complete execution tree or equivalent bounded endpoints. The adapter must flatten nested Categories, Tasks, Subtasks, and Sprints before presenting candidates to Jev.

A common bug is reading only top-level Categories and losing nested candidates. The extractor must recursively traverse at least:

```text
Project.categories[*]
Category.tasks[*]
Task.subtasks[*]
Task.sprints[*]
Subtask.sprints[*]
```

The candidate extractor should include only useful fields:

```json
{
  "key": "C-10001",
  "kind": "category",
  "project_id": "P-10000",
  "name": "Crash diagnostics",
  "goal": "Own local crash collection, restart recovery, and upload evidence",
  "status": "Open",
  "open_task_count": 2,
  "active_sprint_count": 0,
  "recent_evidence": ["M-42"]
}
```

Candidate rules:

- include stable IDs and short distinguishing descriptions;
- include status and runnable/held/terminal state;
- include parent IDs for validation, not as a free-form model invention surface;
- cap candidate count and description length;
- include `create_new`, `none_of_the_above`, and `needs_review` where appropriate;
- do not include credentials, private prompts, raw source, full logs, or secrets;
- do not send the entire database to Jev;
- preserve a hash of the exact bounded candidate state for auditing.

### Step 3: Decide Category ownership

The first Jev call answers only the Category-level routing problem. It should not simultaneously create a Sprint. Category selection is a prerequisite for safe Task/Sprint candidate narrowing.

Conceptual typed question:

```json
{
  "id": "category_target",
  "type": "choice",
  "instructions": "Select the single Project Category that should own this work. Choose create_new only when no existing candidate is an appropriate durable owner. Choose needs_review when the intent is ambiguous or the evidence conflicts.",
  "criteria": {
    "C-10001": "Crash diagnostics; local crash collection, restart recovery, and upload evidence.",
    "C-10002": "API availability; service outages, health checks, and process supervision.",
    "create_new": "No existing Category is an appropriate owner.",
    "needs_review": "Insufficient or conflicting evidence; do not route automatically."
  }
}
```

The Category call should also ask independent supporting questions:

```text
category_confidence_signal: Choice distribution is inspected by policy code
category_requires_new: Noul proposition, if useful for calibration
category_scope_fit: Score from clearly unrelated to exact durable owner
```

Do not ask Jev to emit a new Category name as the authoritative answer in the same field as an existing ID. If `create_new` wins, a separate deterministic proposal field may carry a suggested name and goal, but those strings require review and normal validation.

### Step 4: Apply Category policy

The policy composer inspects:

- selected candidate;
- full probability distribution;
- returned confidence where available;
- difference between first and second candidates;
- explicit `needs_review`/`none_of_the_above` result;
- Project ownership and status;
- whether the work is merely a continuation of an existing Task/Sprint;
- whether creating a new Category is justified.

Initial thresholds are calibration placeholders:

```text
Existing Category suggestion, read-only: confidence >= 0.70
Existing Category used for ordinary work: confidence >= 0.85
New Category proposal: confidence >= 0.90 and no existing candidate > 0.50
Ambiguous or conflicting distributions: review regardless of raw confidence
```

These thresholds must not be treated as scientifically validated until measured against labelled routing decisions.

The output of this step is one of:

```text
category_proposal(existing C-...)
category_proposal(create_new, suggested_name, suggested_goal)
needs_review(category candidates + rationale)
```

No mutation occurs during Category selection in Stage 1.

### Step 5: Narrow Task and Sprint candidates deterministically

If the Category proposal selects an existing Category, the adapter now filters candidates to that Category's Tasks and their Sprints. Jev must not be asked to select a Task from a candidate set that includes unrelated Categories when the adapter can narrow it safely.

This is a separate pass because TypeSafe questions in one request are evaluated in parallel and cannot see each other's answers. A Task question in the first request cannot condition its candidate list on the Category answer.

If the Category proposal is `create_new`, automatic Task/Sprint creation must stop in the initial implementation. The system should require review to approve the new Category and its proposed first Task/Sprint. This prevents a single uncertain answer from creating an entire hierarchy.

The narrowed candidate set should include:

```json
{
  "key": "T-10005",
  "kind": "task",
  "category_id": "C-10001",
  "name": "Crash reporting upload",
  "goal": "Persist and upload crash evidence after restart",
  "status": "Open",
  "sprints": [
    {
      "key": "SP-10006",
      "name": "Implement upload retry",
      "status": "Open",
      "runnable": true,
      "remaining_scope": "..."
    }
  ]
}
```

The adapter must calculate `runnable` using authoritative status and parent state. Jev may recommend a candidate, but the Store decides whether it can actually be started.

### Step 6: Decide Task and Sprint disposition

The second Jev call answers the execution-destination problem within the selected Category.

The primary action question should be a Choice:

```json
{
  "id": "work_disposition",
  "type": "choice",
  "instructions": "Choose what should happen to the intent within the selected Category. Resume an existing runnable Task/Sprint when it clearly matches. Choose create_task_and_sprint only when no existing Task owns the work. Choose needs_review for ambiguity, blocked parents, or insufficient evidence.",
  "criteria": {
    "resume_existing": "Continue an existing matching runnable Task or Sprint.",
    "create_sprint_on_existing_task": "The Task is the correct owner and has capacity for a new bounded Sprint.",
    "create_task_and_sprint": "No existing Task is a suitable owner; propose a new Task and one bounded Sprint.",
    "needs_review": "Do not mutate because the destination or scope is uncertain."
  }
}
```

A separate target question must select the actual existing destination:

```json
{
  "id": "existing_work_target",
  "type": "choice",
  "instructions": "If resuming existing work, select the single Task or runnable Sprint that owns the intent. Select none_of_the_above when no candidate fits.",
  "criteria": {
    "SP-10006": "Implement upload retry; Open and runnable.",
    "T-10007": "Crash reporting dashboard; Open Task with no matching Sprint.",
    "none_of_the_above": "No existing candidate is a safe match.",
    "needs_review": "Candidate evidence is insufficient or conflicting."
  }
}
```

A target question is mandatory whenever an action implies a target. Do not allow a response such as `resume_existing` with no selected Task/Sprint to become an executable proposal.

For a new Sprint estimate, use a bounded Score rather than an unbounded free-form number:

```json
{
  "id": "sprint_size",
  "type": "score",
  "instructions": "Choose the smallest bounded Sprint size that can produce one independently verifiable outcome. If the work is larger than the maximum level, split it into multiple Sprints instead of choosing a larger magnitude.",
  "criteria": [
    "15 minutes: one small change or focused inspection",
    "30 minutes: one bounded implementation plus focused verification",
    "45 minutes: one bounded implementation with integration verification",
    "60 minutes: maximum single Sprint; larger work must be split"
  ]
}
```

Other supporting questions may include:

```text
requires_subtask: Noul; if yes, route to review until Subtask creation is supported
can_start_now: Noul; false when blocked by parent state or external dependency
needs_code_snapshots: Noul; true when source files will change
risk_level: Score; informational -> ordinary -> consequential -> high-risk
```

Jev should never return a raw `estimated_duration_seconds` without policy code bounding it to the supported range. The domain's maximum is a split signal, not an invitation to create a giant Sprint.

### Step 7: Compose a proposal in deterministic code

The composer combines answers and creates a typed proposal. Jev's prose is never parsed as a command.

Example proposal:

```json
{
  "version": "timekeeper-jev-routing-proposal/v1",
  "decision_id": "DEC-20260925-0001",
  "project_id": "P-10000",
  "intent_hash": "sha256:...",
  "action": "resume_existing",
  "category_id": "C-10001",
  "task_id": "T-10005",
  "sprint_id": "SP-10006",
  "estimated_duration_seconds": null,
  "requires_confirmation": false,
  "confidence_gate": {
    "category": 0.91,
    "work_disposition": 0.88,
    "target": 0.90,
    "policy_version": "jev-routing-policy/v1"
  },
  "provenance": {
    "agent_id": "agent:worker-7",
    "session_id": "session-42",
    "shadowbox_run_id": "run-...",
    "model": "jev-latest"
  },
  "idempotency_key": "jev:session-42:intent-sha256:..."
}
```

The composer must reject proposals when:

- Category and Task do not share the same Project;
- Task and Sprint do not share the selected parentage;
- the action requires an existing target but no target was selected;
- the target is not in the candidate snapshot;
- selected candidate status is terminal or non-runnable for a resume action;
- a new Sprint would exceed the one-to-four direct-Sprint limit;
- a new Sprint estimate is outside the bounded range;
- Jev returned missing, blank, malformed, or unknown answers;
- probability distributions do not sum within the accepted tolerance;
- independent answers contradict one another;
- a human confirmation is required but absent;
- a retry would reuse an idempotency key for a different intent;
- project state changed since candidate discovery and the refresh has not occurred.

### Step 8: Re-resolve and mutate through TimeKeeper

Immediately before any write, the adapter refreshes the selected Project tree or calls the relevant read endpoint. Candidate IDs are not trusted merely because Jev selected them earlier.

The mutation path must use existing validated TimeKeeper API routes. Jev and ShadowBox never write SQLite.

Examples:

Resume an existing Sprint:

```text
POST /api/v1/sprints/SP-10006/start
```

Create a Sprint on an existing Task:

```text
POST /api/v1/tasks/T-10005/sprints
Content-Type: application/json
Idempotency-Key: jev:session-42:intent-sha256:...

{
  "name": "Implement crash upload retry",
  "estimated_duration_seconds": 1800,
  "buffer_pct": 10
}
```

Create a new Task and then Sprint only through a transaction-like orchestration with durable idempotency. If the current API cannot atomically create both, persist the proposal as `proposed` and require the adapter to reconcile each step by readback. Do not pretend a multi-step partial application is atomic.

The actual server routes and request bodies must be verified against `internal/api/server.go`, model validation, and Store methods before implementation. In particular, Sprint creation is under `/api/v1/tasks/{taskID}/sprints` or `/api/v1/subtasks/{subtaskID}/sprints`, not under a guessed `/categories/{id}/sprints` path.

After mutation, read back the exact target and verify:

- ID and parent IDs match the proposal;
- status is the expected state;
- name/estimate are the intended bounded values;
- event history contains the mutation;
- no duplicate entity was created;
- the idempotency key maps to this result.

Only after readback may the outcome be recorded as `applied`.

## 6. TypeSafe request contract

The live TypeSafe request must follow the released TypeSafe API/SDK contract. The following is the required semantic shape, not a claim that this exact JSON has already been tested against the live service:

```json
{
  "state": {
    "protocol": "timekeeper-jev-routing/v1",
    "project": {
      "id": "P-10000",
      "name": "Example-Project",
      "status": "Open"
    },
    "intent": {
      "text": "Fix crash diagnostics upload",
      "source": "cli",
      "intent_hash": "sha256:..."
    },
    "candidate_scope": "category",
    "categories": [
      {
        "key": "C-10001",
        "name": "Crash diagnostics",
        "goal": "Own crash collection and upload evidence",
        "status": "Open"
      },
      {
        "key": "C-10002",
        "name": "API recovery",
        "goal": "Own service health and process recovery",
        "status": "Open"
      },
      {
        "key": "create_new",
        "name": "Create a new Category",
        "goal": "No existing Category fits"
      },
      {
        "key": "needs_review",
        "name": "Needs review",
        "goal": "Do not choose automatically"
      }
    ],
    "constraints": {
      "category_must_belong_to_project": true,
      "max_direct_sprints_per_parent": 4,
      "sprint_duration_min_seconds": 900,
      "sprint_duration_max_seconds": 3600
    }
  },
  "model": "jev-latest",
  "questions": {
    "category_target": {
      "type": "choice",
      "instructions": "Select the best existing Category or an explicit escape option.",
      "criteria": {
        "C-10001": "...",
        "C-10002": "...",
        "create_new": "...",
        "needs_review": "..."
      }
    }
  }
}
```

TypeSafe primitives:

- `Choice`: fixed alternatives; inspect selected choice, probabilities, and confidence;
- `Score`: ordered levels; inspect score, legend, probabilities, and confidence;
- `Noul`: probability of one proposition; do not expect a separate confidence field.

Question IDs are for code correlation. The full semantic meaning must be in `instructions` and criteria; Jev must not be expected to infer domain meanings from opaque IDs.

Independent questions in one request cannot see each other's answers. Use two-pass routing for hierarchy-dependent decisions, and use deterministic code to compose results.

## 7. Candidate preparation rules

Candidate preparation is the adapter's highest-leverage deterministic responsibility. Jev cannot choose an entity that was omitted from the state.

### 7.1 Flatten before sending

Use a recursive extractor over the execution tree. Test it with a fixture containing at least:

```text
Project
  Category A
    Task A1
      Sprint A1-1
      Subtask A1-S1
        Sprint A1-S1-1
  Category B
    Task B1
```

The fixture must prove that all depths arrive in candidate state. A live database with only flat Categories is not enough to test this.

### 7.2 Candidate escapes

Every forced Choice must have an escape appropriate to the action:

```text
create_new
none_of_the_above
needs_review
```

Do not offer `create_new` for a read-only resume question if creating a new entity is not allowed in that decision pass; use `none_of_the_above` and route to the creation policy.

### 7.3 Status and eligibility

Candidate summaries must distinguish:

```text
Open and runnable
Open but blocked by parent
Active
On Hold
Completed
TimedOut
Cancelled
```

A completed or held candidate may be useful as historical context but must not be offered as a runnable resume target. A held Sprint may be offered to an explicit resume policy only when that action is supported and its reason is visible.

### 7.4 Bounded state

Set explicit limits for:

- maximum Categories in one Jev request;
- maximum Tasks per Category;
- maximum Sprints per Task;
- maximum characters per name, goal, recent evidence, and intent;
- maximum recent events/checkpoints;
- maximum total serialized state bytes.

If the candidate set exceeds the limit, use deterministic narrowing first: exact Project scope, open status, recent/active work, lexical or embedding prefilter, then `needs_review` if meaningful candidates are still tied. Never silently truncate without recording that truncation occurred.

## 8. Confidence, abstention, and policy gates

Confidence is an uncertainty signal, not proof of correctness. The policy layer must inspect the entire distribution and candidate margin rather than only a single scalar.

Recommended initial gates:

```text
Category suggestion only:
  >= 0.70 may be displayed as the leading suggestion.

Use existing Category for ordinary routing:
  >= 0.85, no contradiction, valid candidate, readback required.

Resume existing Sprint:
  >= 0.85 for target and disposition, target runnable, no conflict.

Create Sprint on existing Task:
  >= 0.90, Task candidate valid, parent open, capacity available,
  bounded estimate, confirmation required in Stage 1.

Create new Task and Sprint:
  >= 0.90 plus explicit confirmation in Stage 1.

Create new Category:
  >= 0.90, no existing Category above 0.50, explicit confirmation always.

Hold, cancel, close, delete, restore, deploy, spend, or alter history:
  Jev never acts alone; explicit confirmation and normal API policy required.
```

The final decision must also account for risk. A high-confidence low-risk Category suggestion may be automatic earlier than a high-confidence new Category creation. The threshold is not a substitute for action policy.

### 8.1 Conflict detection

Because questions are parallel, responses can conflict. Examples:

```text
work_disposition = resume_existing
existing_work_target = none_of_the_above
```

or:

```text
category_target = C-10001
category_requires_new = true
```

or:

```text
create_sprint_on_existing_task
existing_work_target = SP-... from another Category
```

These are incoherent proposals. The composer must route them to review, not pick the most convenient answer.

### 8.2 Missing answers

Absent, blank, unknown, or malformed answers become:

```text
confidence = 0
outcome = needs_review
```

Never fall through to the first candidate or a hardcoded default.

### 8.3 Calibration

Build a labelled evaluation set from real routing examples:

```text
intent
project
candidate Category list
human-selected Category
human-selected Task/Sprint disposition
human rationale
risk level
```

Measure:

- top-1 Category accuracy;
- top-3 coverage;
- abstention rate;
- false-routing rate;
- new-Category false-positive rate;
- existing-Sprint resume accuracy;
- Sprint estimate calibration;
- average latency and cost;
- stale-candidate rejection rate;
- human correction frequency.

Tune criteria descriptions, candidate formatting, deterministic narrowing, and thresholds using held-out examples. A system that always abstains is not calibrated; it is under-specified.

## 9. Decision record and audit trail

Initially, use a compact Project message/checkpoint plus ShadowBox run metadata if a dedicated decision table is not yet implemented. Do not put the full bounded state or raw model response into a 1000-character Project message.

A future first-class decision record should contain:

```text
decision_id
protocol_version
project_id
agent_id
session_id
shadowbox_run_id
intent_hash
candidate_state_hash
candidate_truncated
category_action
category_id
category_probability_json
category_confidence
disposition
selected_task_id
selected_subtask_id
selected_sprint_id
sprint_estimate_seconds
answer_json_bounded
model_version
policy_version
thresholds_json
outcome: proposed | confirmed | applied | reviewed | rejected | failed
idempotency_key
error_code
created_at
applied_at
```

Do not retain:

- TypeSafe API keys;
- bearer tokens;
- private prompts that are not needed for audit;
- raw full transcripts;
- unbounded source files or logs;
- sensitive personal data unrelated to routing.

Store a bounded redacted summary and hashes when full content is unnecessary.

The record must link the decision to:

```text
TimeKeeper Project event
Category/Task/Sprint mutation event, if applied
ShadowBox run ID
agent/session identity
M-<id> evidence or review checkpoint
```

The applied outcome must be verified by reading the exact TimeKeeper record back. A successful HTTP status alone is not proof that the intended entity exists.

## 10. ShadowBox integration and sandbox boundary

The Jev call belongs behind ShadowBox because the TypeSafe credential and outbound network policy belong there, not in TimeKeeper Core.

Required boundary:

```text
TimeKeeper adapter
  -> predetermined ShadowBox connector
  -> ShadowBox decision task
  -> TypeSafe API
  -> typed response
  -> sanitized result back to adapter
```

ShadowBox policy must enforce:

- one allowlisted TypeSafe destination;
- HTTPS only;
- no arbitrary destination supplied by Jev or the intent;
- the credential is supplied to the ShadowBox Jev adapter as `TYPESAFE_API_KEY`;
- ShadowBox owns the secret boundary and resolves `TYPESAFE_API_KEY` from its
  local secret configuration; TimeKeeper does not own or rename the credential;
- credential absent from arguments, state, artifacts, logs, and error bodies;
- bounded request/response size;
- timeout and retry cap for 429/529 responses;
- no shell execution for the decision task;
- no direct TimeKeeper SQLite access;
- no direct mutation API authority unless a separate predetermined connector is explicitly approved;
- sanitized decision artifact with model/version, latency, answer schema, and hashes;
- kill switch and failure state when policy cannot be enforced.

Installation, verification, activation, execution, and publication remain separate. A signed Jev adapter package is not automatically trusted.

If ShadowBox is unavailable, TimeKeeper must remain usable through manual Category/Task/Sprint selection and the existing API/CLI. Jev enrichment must not become a single point of failure for ordinary work tracking.

## 11. API and CLI surface to implement

The first implementation should expose one ergonomic adapter surface and preserve the low-level API.

### 11.1 CLI target

```bash
tk log --project P-10000 "Fix crash diagnostics upload"
tk log --project Example-Project "Fix crash diagnostics upload"
tk log "Fix crash diagnostics upload"   # when a session Project is bound
```

Read-only proposal mode should be explicit during rollout:

```bash
tk log --project P-10000 --propose "Fix crash diagnostics upload"
tk log --project P-10000 --confirm "Fix crash diagnostics upload"
```

The command should print:

```text
Decision DEC-...
Project: P-10000 Example-Project
Category: C-10001 Crash diagnostics (0.91)
Disposition: resume_existing (0.88)
Target: SP-10006 Implement upload retry (0.90)
Action: proposal only / applied
Reason: bounded sanitized summary
Next: start Sprint and record evidence
```

It must print review instructions, not pretend to apply a mutation when confidence is insufficient.

### 11.2 Optional API surface

If server-side orchestration is required, keep it narrow:

```text
POST /api/v1/projects/{projectID}/routing/decide
POST /api/v1/projects/{projectID}/routing/apply
GET  /api/v1/projects/{projectID}/routing/decisions/{decisionID}
GET  /api/v1/projects/{projectID}/routing/decisions
```

The `decide` route should be read/propose only. The `apply` route must require a decision ID, policy version, idempotency key, and confirmation state. It must re-read candidates before mutation.

A first implementation may keep this entirely in the CLI/adapter to avoid adding a premature Core route. If a route is added, it must not make Core call TypeSafe directly or store the API key.

### 11.3 Proposed request envelope

```json
{
  "intent": "Fix crash diagnostics upload",
  "agent_id": "agent:worker-7",
  "session_id": "session-42",
  "mode": "propose",
  "idempotency_key": "jev:session-42:intent-sha256:...",
  "context": {
    "change_summary": "Crash upload retries are missing after restart",
    "evidence_refs": ["M-42"],
    "source": "cli"
  }
}
```

The server or adapter must reject unknown fields, overlong strings, missing project scope, and unverified redaction metadata where raw context is accepted.

## 12. Existing planning drafts and Jev: do not conflate them

TimeKeeper currently has planning-draft routes:

```text
GET  /api/v1/llm-pipelines
POST /api/v1/llm-pipelines
POST /api/v1/projects/{projectID}/planning-drafts/generate
POST /api/v1/projects/{projectID}/planning-drafts
GET  /api/v1/projects/{projectID}/planning-drafts
POST /api/v1/projects/{projectID}/planning-drafts/{draftID}/apply
```

The current planning generator asks an LLM for a `timekeeper-planning-draft/v1` JSON object containing Categories, Tasks, Sprints, and Subtasks, stores it as a draft, and applies it through Store validation.

That feature is useful for broad initial planning. It is not automatically the right mechanism for Jev category/Sprint routing because:

- it generates a whole plan rather than answering bounded atomic routing questions;
- its current pipeline abstraction is not the Jev/ShadowBox security boundary;
- it does not by itself provide the confidence gates and abstention rules defined here;
- applying a planning draft can mutate several hierarchy levels and therefore needs its own explicit confirmation policy;
- a planning draft does not prove that a short work intent should resume a specific existing Sprint.

The two systems may share candidate extraction, schema validation, idempotency, and readback code, but their decision protocols must remain distinguishable in audit records.

## 13. Failure behavior

### Jev unavailable

Return a deterministic manual-review result. Do not block basic TimeKeeper usage. The agent can select Category/Task/Sprint through the normal CLI/API.

### ShadowBox policy denial

Persist the proposal as unexecuted or return a structured policy error. Do not retry around the denial.

### Timeout or 429/529

Retry only transient TypeSafe failures with bounded exponential backoff and a total deadline. After the limit, return `needs_review` or manual fallback. Never duplicate a mutation because the decision call timed out.

### Invalid Jev response

Reject it. Preserve a bounded sanitized error and response hash. Do not attempt to parse prose or guess missing fields.

### Candidate changed after decision

Refresh authoritative state once. If the selected ID is still valid and the idempotency key has not been used, recompute or apply under the refreshed state. If the parent/status changed, create a new decision or route to review. Never apply a stale Category/Task/Sprint selection blindly.

### Duplicate retry

The same idempotency key and same intent/context hash returns the original decision/outcome. The same key with a different hash is a conflict and must not mutate.

### Partial multi-step apply

If Task creation succeeds but Sprint creation fails, record the partial outcome. Do not report “failed with nothing changed.” The reconciler should read back the created Task by idempotency key or durable name and offer a safe retry for only the missing step.

### Low confidence or contradiction

Return review with candidates, distributions, and a compact reason. Do not silently select the nearest Category.

### Human question required

Keep the proposal durable and mark it `reviewed`/`needs_input` only after the human decision is actually received and recorded. The agent should not return to an unexplained waiting state; it should leave a clear blocker in TimeKeeper.

## 14. Implementation stages

### Stage 0: Contract and fixtures

Deliverables:

- this protocol accepted as the implementation source;
- typed proposal and answer structs;
- recursive execution-tree flattening;
- bounded/redacted state builder;
- fixture hierarchy with nested Subtasks and Sprints;
- labelled Category/Task/Sprint routing examples;
- deterministic policy tests with no external credentials;
- explicit redaction and size limits.

Acceptance:

- every candidate has stable ID and correct parent;
- no cross-Project candidate survives extraction;
- missing/blank answers route to review;
- no mutation occurs.

### Stage 1: Advisory local adapter with mocked Jev

Deliverables:

- `tk log --propose` or equivalent adapter command;
- mocked Jev client returning typed Choice/Score/Noul answers;
- Category proposal and second-pass Task/Sprint proposal;
- full probability/threshold display;
- confirmation required for every mutation;
- compact decision evidence in Project messages or a draft record;
- idempotency key generation.

Acceptance:

- clear existing Category/Sprint produces the expected proposal shape;
- ambiguous candidates produce review;
- create-new escapes are visible;
- stale candidate is rejected on refresh;
- repeated proposal does not mutate.

### Stage 2: Jev in ShadowBox

Deliverables:

- minimal TypeSafe HTTP client or SDK behind ShadowBox;
- secret injection and redaction tests;
- allowlisted egress and request/response limits;
- schema validation for Choice/Score/Noul responses;
- bounded retries/timeouts;
- ShadowBox run ID and sanitized artifact.

Acceptance:

- TimeKeeper Core remains offline-capable;
- no TypeSafe secret reaches Core logs or project messages;
- ShadowBox cannot write SQLite;
- policy denial is fail-closed;
- live call can be disabled without breaking manual workflow.

### Stage 3: Evaluation and calibration

Deliverables:

- replay harness over labelled work intents;
- accuracy, abstention, false-route, latency, cost, and estimate calibration report;
- held-out evaluation set;
- threshold/version configuration;
- human correction capture.

Acceptance:

- thresholds are based on observed outcomes, not intuition;
- the system can explain why it abstained;
- model upgrades can be compared by protocol/version.

### Stage 4: Confidence-gated automation

Deliverables:

- automatic existing Category routing above calibrated threshold;
- automatic resume of an existing runnable Sprint where risk policy permits;
- confirmation for new Category, new Task, and new Sprint at first release;
- full mutation readback and decision record;
- rollback/reconciliation procedure.

Acceptance:

- no duplicate hierarchy rows under retries;
- every applied decision has matching durable event/readback;
- a Jev outage falls back to manual operation;
- high-risk actions cannot be automated by threshold alone.

## 15. Test plan

### Candidate extraction

- flatten three or more hierarchy levels;
- include direct Task Sprints and Subtask Sprints;
- exclude foreign Project entities;
- preserve parent IDs;
- flag blocked/held/terminal status accurately;
- enforce candidate count and text limits;
- record truncation in the state hash/envelope.

### Jev response validation

- valid Choice with probabilities and confidence;
- valid Score with ordered legend;
- valid Noul with only yes-probability;
- missing answer;
- blank answer;
- unknown answer key;
- invalid probability sum;
- malformed score or out-of-range value;
- oversized response;
- secret-containing error body redaction.

### Policy composition

- clear Category winner;
- close Category candidates;
- `create_new` with no candidate above threshold;
- disposition/target contradiction;
- Category/Task parent mismatch;
- existing runnable Sprint resume;
- existing Task with Sprint capacity;
- Task at four-Sprint limit;
- new Category always requires review/confirmation;
- estimate above maximum becomes split/review;
- risk gate overrides high raw confidence;
- no target for target-requiring action.

### Mutation and consistency

- idempotent apply retry returns the original outcome;
- same idempotency key with different intent conflicts;
- stale candidate refresh rejects or recomputes;
- parent closes between proposal and apply;
- Sprint creation uses the actual Task/Subtask route;
- Sprint cannot complete before start;
- apply readback matches proposal;
- partial Task/Sprint application is recorded honestly;
- no Jev/ShadowBox component writes SQLite directly.

### Integration and live behavior

- mock ShadowBox unavailable;
- TypeSafe 401 fails without retry;
- 429/529 retry within a bounded deadline;
- policy denial fails closed;
- no secret in normal/debug logs;
- `go test ./...` and `go build ./...` pass;
- live tests assert response contract, not a permanently fixed model choice;
- live mutation tests require an explicit scratch Project and unique idempotency/name scope.

## 16. Operational debugging

When a routing result is wrong, inspect in this order:

1. Did the project name resolve to the correct canonical `P-` ID?
2. Did candidate extraction include the correct Category/Task/Sprint?
3. Was the candidate state truncated or stale?
4. Did Jev return a valid typed response and distribution?
5. Did the policy threshold/margin/abstention gate behave as configured?
6. Did deterministic composition detect answer contradictions?
7. Did TimeKeeper revalidate parentage/status before mutation?
8. Did the mutation route match the actual server route table?
9. Did readback confirm the intended row and parent?
10. Was the decision/outcome recorded with the correct idempotency key?

Do not debug by staring only at Jev's prose. The useful evidence is the bounded candidate snapshot, typed answer, policy decision, TimeKeeper validation response, and readback.

Use normal logging for command, decision ID, stage, status, latency, and outcome. Debug logging may include bounded redacted request/response metadata, but never raw credentials or unrestricted state.

## 17. Source map

```text
docs/jev-typesafe-timekeeper-integration.md
  Original conceptual design exploration; this protocol supersedes it for
  implementation detail while preserving its authority boundary.

docs/AGENT_INTEGRATION.md / AGENT_INTEGRATION.md
  Low-level TimeKeeper API and intended ergonomic tk log contract.

docs/queue-logic.md
  Durable hierarchy, Sprint lifecycle, queue eligibility, and 1–4 Sprint rule.

internal/api/server.go
  Current route table, planning draft routes, Category/Task/Sprint handlers.

internal/store/
  Actual validation, parentage, status transitions, idempotency, and persistence.

internal/model/
  Typed Project, Category, Task, Subtask, Sprint, planning, and event models.

internal/planning/
  Existing LLM planning-draft client; not the Jev decision boundary.

ShadowBox decision adapter (target)
  TypeSafe client, secret scope, response validation, policy boundary, run ID,
  redaction, timeout/retry, and sanitized result artifact.
```

## 18. Definition of done

Jev is usable as a Category/Sprint decision maker only when all of these are true:

```text
[ ] Project resolution is deterministic and exact.
[ ] Execution-tree candidates are complete, bounded, redacted, and parent-linked.
[ ] Category decision and Task/Sprint decision are separate typed passes.
[ ] Every forced choice has an abstention/create-new/review escape.
[ ] Target-requiring actions always select an actual target.
[ ] Jev answers are schema-validated; prose is never executed.
[ ] Policy code owns thresholds, risk, contradiction detection, and confirmation.
[ ] ShadowBox owns TypeSafe secrets and default-deny egress.
[ ] TimeKeeper Core remains the only durable authority and mutation path.
[ ] Candidate IDs are revalidated immediately before mutation.
[ ] Mutations use idempotency keys and are read back exactly.
[ ] New Category and high-risk actions require explicit confirmation.
[ ] Sprint estimates are bounded; oversized work is split or reviewed.
[ ] Decision and outcome evidence are durable and sanitized.
[ ] Jev outage/timeout/policy denial falls back to manual operation.
[ ] Calibration data measures accuracy, abstention, false routing, and cost.
[ ] Tests cover contradictions, stale state, retries, partial apply, and secrets.
[ ] No documentation claims the live integration exists before these gates pass.
```

The practical rule is simple: Jev may recommend where work belongs and how it should be bounded. TimeKeeper decides whether that recommendation is structurally valid, authorized, and durable. ShadowBox keeps the model call and its secret behind a controlled boundary. No clever model answer gets to skip the boring parts. That is precisely why the system remains trustworthy.
