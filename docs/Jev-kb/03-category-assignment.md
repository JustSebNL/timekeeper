# Jev KB — Assigning Work to the Correct Category

## Objective

Select the Category that durably owns the work inside one already-resolved Project. Category selection is about ownership and durable grouping, not a temporary label.

## Category candidate facts

Send compact candidates containing:

```text
category_id
project_id
parent_category_id (if nested)
name
goal
description
status
priority
progress
child task summaries
recent relevant evidence references
```

Example:

```json
{
  "key": "C-10001",
  "kind": "category",
  "project_id": "P-10000",
  "parent_category_id": null,
  "name": "Download queue",
  "goal": "Own task containers, Sprint execution lanes, and queue behavior",
  "description": "...",
  "status": "Open",
  "priority": "High"
}
```

Flatten nested Categories recursively. A child Category is a real candidate; do not stop at the first level.

## Assignment rubric

Evaluate candidates against these dimensions:

1. **Ownership:** Does the Category's goal explicitly cover this kind of work?
2. **Scope:** Is the intent about the Category's durable area rather than a neighboring concern?
3. **Artifact alignment:** Do the named files, UI surface, API, or subsystem belong there?
4. **Continuity:** Is there an existing Task/Sprint in this Category already working on the same outcome?
5. **Boundary:** Would assigning here mix two products or projects?
6. **Status:** Is the Category open and eligible for new work?
7. **Specificity:** Is there a narrower child Category that owns the work better?

Prefer the narrowest existing Category whose goal covers the complete outcome. Do not choose a broad parent when a suitable open child Category exists.

## Category decision options

Every forced decision must include:

```text
existing C-...
create_new
none_of_the_above
needs_review
```

Use:

- `existing`: a current Category clearly owns the work;
- `create_new`: no existing Category can own it without distorting its scope;
- `none_of_the_above`: candidate list is insufficient or no safe existing owner fits;
- `needs_review`: ambiguity, conflicting evidence, cross-project concern, or status problem.

Do not force a Category because it is the least-wrong candidate.

## When to create a Category

A new Category is justified only when all are true:

```text
No existing Category has a durable scope fit.
The work is a recurring or meaningful area, not a one-off detail.
A new Category will not duplicate an existing parent/child Category.
The proposed name and goal are specific and distinguishable.
The selected Project is correct and Open.
Policy threshold and human confirmation requirements pass.
```

A single bug or Sprint normally belongs in an existing Category. Suggesting a new Category is a high-impact organizational change and requires review/confirmation in the initial rollout.

## Nested Category rules

- `parent_category_id` must refer to a Category in the same Project.
- Do not create a child Category merely to make a Task name fit.
- If a parent Category already has a child with the same durable scope, prefer that child.
- A child Category does not change Project ownership.
- Category status is independent from Jev confidence; a high-confidence candidate that is On Hold or Completed may still be ineligible for new work.

## Typed question template

```json
{
  "id": "category_target",
  "type": "choice",
  "instructions": "Choose the single existing Project Category that durably owns this work. Use create_new only when no existing Category fits. Use needs_review when evidence is ambiguous or conflicting.",
  "criteria": {
    "C-10001": "Download queue: Task containers and Sprint execution lanes.",
    "C-10002": "Crash diagnostics: crash collection and upload evidence.",
    "create_new": "No existing durable Category fits.",
    "needs_review": "Do not route automatically."
  }
}
```

Supporting questions may score scope fit or ask whether a new Category is required, but deterministic policy code owns the final gate.

## Category policy

Initial calibration placeholders:

```text
>= 0.70: may display as a read-only suggestion
>= 0.85: may route ordinary existing-Category work after validation
>= 0.90: new Category proposal still requires explicit confirmation
```

Inspect the complete probability distribution, top-two margin, status, parentage, and contradiction checks. Confidence is not correctness.

## Common mistakes

- Filing Suck-It-Up implementation work under the TimeKeeper Project because TimeKeeper tracks it.
- Choosing a Category by name similarity while ignoring its goal.
- Missing a nested Category because the tree loader only reads top-level nodes.
- Creating a new Category for every bug.
- Treating `create_new` as permission to mutate immediately.
- Sending Category names without IDs and then trying to reconstruct IDs later.
- Assigning to an On Hold or Completed Category without an explicit policy.

## Output

```json
{
  "action": "existing_category | create_new_category | needs_review",
  "project_id": "P-10000",
  "category_id": "C-10001",
  "parent_category_id": null,
  "confidence": 0.89,
  "top_alternatives": [
    {"category_id": "C-10002", "probability": 0.07}
  ],
  "reason": "The Category goal explicitly owns the named queue surface.",
  "requires_confirmation": false
}
```

Do not emit `category_id` for `create_new_category` unless TimeKeeper has already created it and readback verified it.
