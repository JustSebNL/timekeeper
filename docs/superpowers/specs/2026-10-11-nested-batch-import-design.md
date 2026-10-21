# Nested execution batch import design

## Purpose

Add a safe, documented batch workflow for creating Tasks, Subtasks, and Sprints
inside an existing TimeKeeper Project. Existing task-only batch formats must
continue to work. Every batch must pass a real smoke test before the Core
commits changes; a failed check must leave the database unchanged.

This design also replaces the ad-hoc shape used by
`docs/Extended-Web-UI-Migration-Objects.json` with a versioned executable
format while retaining migration-phase mapping and deferred-work information
as non-executable metadata where useful.

## Current behavior and constraints

- `tk t batch <project-id> <file>` currently parses task-only JSON/YAML and
  other supported task formats, then calls the atomic Task batch endpoint.
- The task batch store method creates its rows in one transaction. Individual
  Subtask and Sprint methods each open their own transaction, so composing them
  from the CLI could leave a partially created hierarchy.
- Full Project import understands a hierarchy but always creates a new Project;
  it is not suitable for applying work beneath the migration Project.
- Sprint ownership and parent-open rules are enforced by Core. Each Task or
  Subtask may own at most four direct Sprints, including terminal Sprints.
- The recent migration object manifest is not an input accepted by the current
  task importer, which only understands a `tasks` array or a bare task array.

## Chosen approach

Add a versioned execution-batch contract and a Core-owned atomic persistence
path. Preserve the existing task-only API and formats. Add read-only batch
validation and atomic batch creation endpoints. The CLI runs the smoke test
first; its checkmark is the conditional gate: only a passing result proceeds to
apply. A `--check-only` mode provides the same validation/checkmark without
applying anything. The apply operation repeats all database-dependent checks
inside the creation transaction so intervening state changes cannot bypass
ownership, status, or capacity rules.

Other considered approaches:

1. Sequential CLI calls to the existing Task, Subtask, and Sprint APIs are
   simple, but failure partway through leaves a partial tree. Rejected.
2. Reuse full Project import with an existing-project option would mix project
   ID remapping and existing-object mutation semantics into the wrong import
   contract. Rejected.

## Batch document contract

The canonical format is JSON or YAML with `version: timekeeper-execution-batch/v1`.
The Project ID remains an explicit CLI/API target; Categories and existing
parents are referenced by their TimeKeeper IDs. The document separates new
Tasks from additions beneath existing Tasks/Subtasks:

```yaml
version: timekeeper-execution-batch/v1
tasks:
  - category_id: C-11483
    name: Add a sample feature
    goal: Deliver one independently verifiable outcome
    description: Optional implementation context
    priority: Normal
    estimated_duration_seconds: 7200
    auto_close: false
    sprints:
      - name: Implement the Core contract
        goal: Persist and expose the feature
        estimated_duration_seconds: 1800
        buffer_pct: 0
    subtasks:
      - name: Build and verify the UI
        goal: Render the Core-backed feature accessibly
        estimated_duration_seconds: 3600
        sprints:
          - name: Implement the view
            estimated_duration_seconds: 1800
            buffer_pct: 0
extensions:
  - task_id: T-11498
    subtasks:
      - name: Build the Trail of Thought view
        estimated_duration_seconds: 5400
        sprints:
          - name: Implement the view
            estimated_duration_seconds: 1800
            buffer_pct: 0
    sprints: []
  - subtask_id: ST-11549
    sprints:
      - name: Add feed error-state coverage
        estimated_duration_seconds: 1800
        buffer_pct: 0
planning:
  # Informational only: these records never create or modify TimeKeeper items.
  executable_task_metadata:
    - task_index: 0
      key: sample_feature
      phase: 1
      acceptance: ["Smoke test and atomic apply pass"]
  deferred_items:
    - key: future_provider
      condition: "Deferred until its provider contract and approval exist."
      acceptance: ["Provider outage leaves local operation intact."]
  release_gates: []
  application_constraints: []
```

Fields map to existing `CreateTaskInput`, `CreateSubtaskInput`, and
`CreateSprintInput` contracts; the optional `planning` object has only the
documented `executable_task_metadata`, `deferred_items`, `release_gates`, and
`application_constraints` keys and is strictly validated but never executed.
Status, IDs for newly created objects, time
entries, and lifecycle transitions are never accepted from a batch. Estimates
are positive where required, `buffer_pct` is a whole number from 0 to 100, and
omitted optional fields use the same defaults as normal object creation.

Validation rejects unknown fields, an unsupported version, empty/invalid names,
negative or invalid estimates, duplicate/ambiguous owner references, missing or
foreign-project IDs, closed parent chains, and any Task/Subtask that would
exceed four direct Sprints after the batch. The batch is bounded to at most 100
new Tasks and 500 total created objects (Tasks + Subtasks + Sprints). Deferred
or conditional ideas are planning metadata, not executable entries; they must
not be silently converted into Tasks by the importer.

## CLI and smoke-test behavior

- Keep legacy task-only invocation and supported formats compatible.
- For a versioned nested JSON/YAML file, `tk t batch <project-id> <file>`:
  1. Parses the file strictly.
  2. Calls the no-write Core smoke-test endpoint.
  3. Prints a `✓ Smoke test passed` checkmark with counts only on full success.
  4. On PASS, submits the batch for atomic creation; on any failed check, exits
     nonzero and performs no writes.
- `tk t batch <project-id> <file> --check-only` stops after the no-write smoke
  test and checkmark, allowing a person or automation to inspect the result.
- The apply endpoint repeats validation within the same SQLite transaction that
  writes the objects. It returns every created Task/Subtask/Sprint ID and the
  resulting item addresses.
- Do not automatically retry an apply after an ambiguous transport timeout;
  inspect the Project tree first to avoid duplicate imports.
- Legacy CSV/TSV/XLSX/SQL/archive task imports continue using the existing task
  batch path and remain atomic. The smoke-test-before-apply guarantee applies
  to these imports too: their parsed Task payload is validated before commit.

## Core/API boundary

Add a strict nested batch request/response model. Add Project-scoped Core routes
for validation and apply, without changing the meaning of existing
`POST /api/v1/projects/{projectID}/tasks/batch`:

- `POST /api/v1/projects/{projectID}/tasks/batch/validate` performs a read-only
  validation for the legacy task-only payload, so CSV/TSV/XLSX/SQL/archive and
  legacy JSON/YAML imports receive the same pre-apply smoke-test gate.
- `POST /api/v1/projects/{projectID}/execution-batches/validate` performs a
  read-only validation and returns counts plus structured validation errors.
- `POST /api/v1/projects/{projectID}/execution-batches` validates and creates
  all requested objects atomically, returning all created entities.

Store validation and creation share the same hierarchy rules. The write path
uses one transaction and transaction-aware insert helpers rather than calling
the current public create methods that each open a transaction. References to
existing Tasks/Subtasks must be Open and belong to the target Project; their
full parent chain and current Sprint counts are checked in the write
transaction. Any failure rolls back all objects in the batch.

## Error and safety behavior

- Syntax/schema and server validation errors identify the object path (for
  example `tasks[2].subtasks[0].sprints[1]`), the field (for example
  `estimated_duration_seconds`), a concise explanation, and a concrete fix
  (for example, "set this to a positive number of seconds").
- Validation returns all independent, safely discoverable errors in stable
  document order, rather than stopping after the first bad object. If parsing
  cannot safely continue (for example, malformed YAML), report the exact
  document location when available and the syntax correction needed.
- Failed validation prints a clear failure mark, zero committed counts, and
  each issue in a repair-oriented form, such as:

  ```text
  ✗ Smoke test failed; nothing was written (0 objects committed)
  tasks[2].subtasks[0].sprints[1].estimated_duration_seconds:
    must be greater than 0; set it to a positive duration in seconds.
  extensions[0].task_id:
    T-99999 is not an open Task in Project P-10053; choose an open Task ID
    from this Project or add the work beneath a new task in `tasks`.
  ```

- Machine-readable validation responses expose stable `path`, `field`,
  `code`, `message`, and `suggestion` properties per issue. Messages must not
  echo secrets or arbitrary untrusted payloads. A failed check never prints a
  passing checkmark and never calls the apply endpoint.
- Core writes are all-or-nothing. No best-effort partial mode is included.
- The API enforces project/category/parent ownership and lifecycle rules; CLI
  checks are not treated as a security boundary.
- The smoke test itself performs no inserts, counter allocation, or project
  events. Apply-time validation is repeated to close the time-of-check/time-of-
  use gap.

## Documentation and migration artifact

Update `docs/import-export.md` with the new schema, legacy compatibility,
smoke-test/checkmark behavior, `--check-only`, examples, limits, error handling,
and safe retry guidance. Convert
`docs/Extended-Web-UI-Migration-Objects.json` to the canonical contract for
createable Tasks and extensions. Retain phase mapping, acceptance, release
gates, and application constraints only in the non-executable `planning`
object. Conditional/deferred items stay only in `planning.deferred_items` and
must never be counted or created as executable objects.

## Verification

- Parser tests for JSON/YAML, strict unknown fields, versioning, nested entries,
  invalid values, limits, and legacy task-only formats.
- Store tests prove successful hierarchy creation, existing-parent extensions,
  cross-project/status/capacity rejection, and complete rollback when a later
  object fails.
- API tests prove read-only validation does not mutate counts/events, apply
  returns all created IDs, and invalid batches leave the tree unchanged.
- CLI tests prove checkmark only appears on PASS, failed smoke tests never call
  apply, every failure includes object path/field/reason/actionable suggestion,
  multiple independent errors are reported together, `--check-only` never
  writes, and legacy imports still apply atomically.
- Run focused Go tests and the relevant broader suite; run `git diff --check`.
- Source tests do not prove the installed TimeKeeper service has been updated.

## Out of scope

- Importing or merging whole Projects into existing Projects.
- Deleting, updating, or changing workflow state on existing objects.
- Automatic best-effort partial creation, implicit retries, external provider
  provisioning, or changes to the installed service/database.
