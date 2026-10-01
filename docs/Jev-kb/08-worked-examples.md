# Jev KB — Worked Routing Examples and Evaluation Cases

These examples teach decision shape. IDs are illustrative unless they come from the current candidate state. Never copy an example ID into a live mutation.

## Example 1: Select the correct Project

Intent:

```text
Keep Tasks in the Tasks queue while their Sprints run in separate lanes.
```

Candidates:

```text
P-10000 Suck-It-Up
  Goal: Download queue, Task containers, Sprint lanes, drag/drop.

P-10053 TimeKeeper
  Goal: Durable project execution memory and API.
```

Decision:

```text
select P-10000 Suck-It-Up
reason: the intent changes Suck-It-Up product behavior; TimeKeeper is only the tracking authority
```

Do not file the implementation under TimeKeeper merely because TimeKeeper records the work.

## Example 2: Assign an existing Category

Intent:

```text
Fix Sprint lane ordering in the Download tab.
```

Category candidates:

```text
C-1 Download queue: Task containers and Sprint execution lanes
C-2 Crash diagnostics: crash collection and upload evidence
C-3 Account billing: user plan and payment behavior
```

Decision:

```text
existing_category C-1
```

The decision is based on Category goal and artifact scope, not the shared word “Sprint” alone.

## Example 3: Category requires review

Intent:

```text
Improve agent routing and memory handoff.
```

Candidates:

```text
C-1 TimeKeeper agent integration
C-2 Hercules orchestration
C-3 HSAM memory
```

If the intent does not identify which product is being changed, return `needs_review`. Do not select the Category with the highest keyword overlap.

## Example 4: Resume an existing Sprint

Selected Category:

```text
C-1 Download queue
```

Task candidates:

```text
T-10 Keep Task containers durable
  SP-21 Implement queue persistence — Open, runnable
  SP-22 Add regression tests — Open, runnable

T-11 Redesign lane visuals
  SP-23 Polish glass styling — Open, runnable
```

Intent:

```text
Make the queue stop removing Tasks after a Sprint completes.
```

Decision:

```text
resume_existing SP-21
```

Do not create another Task or Sprint. The existing Sprint goal is the same bounded outcome.

## Example 5: Create a Sprint under an existing Task

Task:

```text
T-10 Keep Task containers durable
```

Existing direct Sprints:

```text
SP-21 Completed
SP-22 Completed
```

Intent:

```text
Verify the installed runtime still keeps completed Sprint history while the Task remains visible.
```

Decision:

```text
create_sprint_on_existing_task
name: Verify durable Task container behavior
bounded estimate: focused verification
```

Before creation, check that the Task and all parents are Open and that the Task has fewer than four direct Sprints. Completing SP-21 and SP-22 does not itself complete T-10.

## Example 6: Create a new Task and Sprint

Selected Category:

```text
C-1 Download queue
```

Existing Tasks:

```text
T-10 Task container persistence
T-11 Lane visual styling
```

Intent:

```text
Add an export/import format for queue recovery.
```

If neither Task goal covers durable queue export/import, propose:

```text
create_task
name: Queue recovery export/import
goal: Persist and restore queue state with verified compatibility.
create one bounded Sprint under the new Task
```

Stage 1 requires human confirmation. Do not create the Task and Sprint merely because Jev is confident.

## Example 7: Four-Sprint capacity

Task direct Sprints:

```text
SP-1 Completed
SP-2 Completed
SP-3 Cancelled
SP-4 TimedOut
```

The direct count is still four. Do not create SP-5. The options are:

```text
reuse an eligible existing path only if valid
create a valid Subtask if policy supports it
create a follow-on Task
ask for review
```

Terminal history is not deleted to create capacity.

## Example 8: Parent is not Open

Candidate:

```text
Task T-10: Open
Category C-1: On Hold
```

Even if the Task and intent match perfectly, a new Sprint cannot be created because the Category is not Open. Return review or use the supported resume policy. Never silently reopen C-1.

## Example 9: Held Sprint

Candidate:

```text
SP-25: On Hold
hold_reason: waiting for user approval
```

Intent:

```text
Continue SP-25.
```

Do not automatically resume if approval has not been recorded. Return `needs_review` with the hold reason. A reminder is not approval.

## Example 10: Timeout

Candidate:

```text
SP-30: TimedOut
retrieval_attempts: 4
```

A TimedOut Sprint is terminal and cannot be restarted. Propose a new bounded Sprint only after reviewing why the previous one timed out and ensuring the parent chain/capacity rules pass.

## Example 11: Contradictory answers

```text
work_disposition = resume_existing
existing_work_target = none_of_the_above
```

Output:

```text
needs_review
reason: disposition requires an existing target, but no target was selected
```

Never silently choose the first candidate.

## Example 12: Oversized work

Intent:

```text
Rebuild the entire Download queue architecture, migrate old data, redesign the UI, and add end-to-end tests.
```

Decision:

```text
do not create one giant Sprint
split into independently verifiable Tasks/Sprints
or return needs_review for planning
```

A Score may identify the work as oversized, but policy—not Jev prose—decides the split.

## Evaluation labels

For calibration, store human labels in this shape:

```json
{
  "intent": "Fix Sprint lane ordering",
  "project_id": "P-10000",
  "candidate_category_ids": ["C-1", "C-2"],
  "human_category_id": "C-1",
  "human_disposition": "resume_existing",
  "human_target_id": "SP-21",
  "risk": "ordinary",
  "rationale": "Existing Sprint owns same outcome"
}
```

Measure Category accuracy, correct abstention, false routing, Sprint-target accuracy, capacity errors, stale-state errors, and human corrections. Do not test a live model by requiring one permanent choice value.
