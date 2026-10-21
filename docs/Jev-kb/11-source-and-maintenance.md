# Jev KB — Source, Maintenance, and Verification

## What this knowledge base is

`docs/Jev-kb/` is retrieval-friendly domain knowledge for Jev. It explains concepts, decision rules, examples, and output semantics.

It is not a second database, not a replacement for API validation, and not permission to mutate TimeKeeper directly.

## Authoritative runtime sources

When updating these files, verify against:

```text
internal/model/project.go
internal/model/category.go
internal/model/task.go
internal/model/subtask.go
internal/model/sprint.go
internal/model/execution_tree.go
internal/store/store.go
internal/store/*sprint*.go
internal/store/*category*.go
internal/api/server.go
docs/API.md
docs/queue-logic.md
docs/jev-category-sprint-decision-protocol.md
```

Use the live API when possible:

```text
GET http://timekeeper.local/health
GET http://timekeeper.local/api/help
GET http://timekeeper.local/api/v1/projects/{projectID}/execution-tree
GET http://timekeeper.local/api/v1/projects/{projectID}/operational-summary
GET http://timekeeper.local/api/v1/projects/{projectID}/attention
```

## Update rules

Update the KB when any of these change:

- hierarchy or parentage;
- Project/Category/Task/Subtask/Sprint fields;
- statuses or allowed transitions;
- one-to-four Sprint capacity;
- claim-next ordering or eligibility;
- API routes or request fields;
- FASE actor/claim rules;
- ShadowBox connector contract;
- Jev response schema;
- threshold/policy version;
- audit record shape;
- human confirmation requirements.

Do not silently change a rule in only one KB file. Update the index, affected domain file, output contract, examples, and protocol references together.

## Verification checklist

```text
[ ] Every API route named in the KB exists in internal/api/server.go or is clearly marked proposed.
[ ] Every field named in examples exists in the current model or is marked projection/proposed.
[ ] Category candidates preserve nested parent_category_id.
[ ] Task candidates preserve category_id and Project ID.
[ ] Direct Task and Subtask Sprints remain separate.
[ ] Four-Sprint capacity is described correctly.
[ ] Open-parent requirement is described correctly.
[ ] Sprint lifecycle is not shortened to Open -> Completed.
[ ] Held, TimedOut, Cancelled, and non-runnable states are distinguished.
[ ] No example claims Jev performed a mutation.
[ ] No credential value appears in any KB file.
[ ] No KB rule bypasses Store validation.
[ ] `git diff --check` passes.
```

## Calibration maintenance

Every human correction should become a labelled evaluation example without copying sensitive content. Record:

```text
intent hash
candidate snapshot hash
Jev recommendation
human choice
reason for correction
risk
outcome
protocol/policy version
```

Do not tune thresholds from one anecdote. Review false routing, false abstention, new-Category proposals, Sprint target accuracy, capacity errors, and stale-state errors across a held-out set.

## Retrieval maintenance

Keep files small enough to retrieve independently. Prefer one concept per file. Add cross-links when a decision requires another rule. Do not paste the entire API or source code into every prompt.

Recommended retrieval:

```text
always: 00-index + 01-domain + 07-policy + 09-output
project selection: 02-project
category: 03-category
Task: 04-task
Sprint: 05-sprint + 06-queue
examples/explanation: 08-examples
runtime prompt: 10-runtime-instructions
```

## Security maintenance

The KB must never contain:

```text
TYPESAFE_API_KEY values
bearer tokens
private endpoints
connection strings
raw transcripts
private source dumps
unredacted user data
```

The credential belongs to the ShadowBox runtime configuration. The KB may name the variable `TYPESAFE_API_KEY`, but must not contain its value.
