# Jev Knowledge Base — Index and Retrieval Guide

Status: domain knowledge for the Jev/TypeSafe Category and Sprint decision adapter

This directory teaches Jev how TimeKeeper organizes work and how to make a safe typed recommendation. It is not the authority for IDs, statuses, permissions, or mutations. The live TimeKeeper API and Store are authoritative when this knowledge base and runtime state differ.

## Primary objective

Given a bounded work intent and a resolved Project, recommend:

1. the correct existing Category, or `create_new`/`needs_review`;
2. the correct existing Task or Subtask, or a new Task proposal;
3. whether to resume an existing Sprint, create a new bounded Sprint, or stop for review.

Jev recommends. Deterministic policy code validates. TimeKeeper persists.

## Read order

Read these documents in order when building the full decision context:

```text
01-domain-ontology.md
02-project-selection.md
03-category-assignment.md
04-task-detection.md
05-sprint-detection-and-creation.md
06-queue-and-lifecycle.md
07-decision-policy.md
09-output-contract.md
08-worked-examples.md
```

For a single decision, retrieve only the relevant subset:

| Decision | Required documents |
|---|---|
| Select Project | 01, 02, 07, 09 |
| Assign Category | 01, 02, 03, 07, 09 |
| Detect existing Task | 01, 03, 04, 07, 09 |
| Detect/resume Sprint | 01, 04, 05, 06, 07, 09 |
| Create Task/Sprint proposal | 01, 03, 04, 05, 06, 07, 09 |
| Explain a recommendation | Relevant domain document plus 08 examples |

## Authority order

When sources disagree, use this order:

```text
1. TimeKeeper Store validation and current API response
2. TimeKeeper API route contract
3. Current execution-tree / operational-summary state
4. This Jev-kb knowledge base
5. A model's general assumptions
```

Never invent an ID, parent, status, route, or lifecycle transition from prose.

## Non-negotiable invariants

```text
Project -> Category -> Task -> optional Subtask -> Sprint
Every entity belongs to exactly one Project.
A Sprint belongs directly to a Task or an optional Subtask.
A Task/Subtask owns 1–4 direct Sprints.
Only Open parent chains can create or claim runnable Sprint work.
Open Sprints are started before they are completed.
Completed, TimedOut, and Cancelled history is not deleted.
Held work is not runnable until explicitly resumed.
Jev never writes SQLite or executes commands.
Low confidence, contradiction, stale state, and missing targets become review.
```

## Decision passes

Use sequential passes for hierarchy-dependent choices:

```text
Pass 0: resolve Project deterministically
Pass 1: choose Category
Policy: validate Category and narrow candidates
Pass 2: detect Task/Subtask and choose Sprint disposition/target
Policy: validate target, duration, capacity, risk, and confirmation
TimeKeeper: perform any approved mutation and provide readback
```

Do not ask one parallel Jev request to select a parent Category and a child Sprint from a child list that depends on the parent answer. TypeSafe questions in one request are independent.

## Required input envelope

```json
{
  "intent": "short bounded description of the work",
  "project_ref": "exact Project ID or exact alias",
  "agent_id": "stable actor identity",
  "session_id": "stable session identity",
  "candidate_state": "bounded authoritative execution-tree projection",
  "mode": "propose | confirm | review",
  "provenance": "cli | harness | guardian | human | integration"
}
```

The adapter must add candidate IDs and statuses from TimeKeeper. Jev must not be asked to derive IDs from names alone.

## Required output behavior

Every recommendation must include:

```text
decision_id
selected project/category/task/subtask/sprint IDs where applicable
action: resume_existing | create_task | create_sprint | create_task_and_sprint | needs_review
confidence/probability evidence
short rationale tied to candidate facts
blocking reason when review is required
whether confirmation is required
idempotency key for any proposed mutation
```

A recommendation without a valid target is not executable. A recommendation with `needs_review` is a successful safe decision, not a failure.

## Source map

```text
../jev-category-sprint-decision-protocol.md
  Full implementation blueprint, ShadowBox boundary, thresholds, audit, and tests.
../queue-logic.md
  Durable queue and Sprint lifecycle explanation.
../API.md
  Current routes and response contract.
../../internal/model/
  Runtime entity fields.
../../internal/store/
  Actual validation and mutation authority.
```
