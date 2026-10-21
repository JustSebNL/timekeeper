# Import and export

Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

Task batch import has a format contract and a no-write preflight gate; see [Batch import format](batch-import-format.md) for every accepted legacy and versioned format. Full-project import/export and Task export are separate workflows. The API accepts normalized JSON; the CLI handles file parsing, format conversion, archive unpacking, and document rendering client-side.

## Supported format matrix

| Extension | Format | Task Import | Task Export | Project Import | Project Export |
|---|---|---|---|---|---|
| `.json` | JSON | ✔ | ✔ | ✔ | ✔ |
| `.yaml` / `.yml` | YAML | ✔ | ✔ | ✔ | ✔ |
| `.xlsx` | Excel | ✔ | ✔ | ✔ (multi-sheet) | ✔ (multi-sheet) |
| `.csv` | CSV | ✔ | ✔ | ✔ (ZIP / TAR / RAR) | ✔ (ZIP / TAR.GZ) |
| `.tsv` | TSV | ✔ | ✔ | ✔ (ZIP / TAR / RAR) | ✔ (ZIP / TAR.GZ) |
| `.sql` | SQL | ✔ | ✔ | ✔ | ✔ |
| `.zip` | ZIP Archive | ✔ | ✔ | ✔ | ✔ |
| `.tar` / `.tar.gz` / `.tgz` / `.gz` | Linux Tarball | ✔ | ✔ | ✔ | ✔ |
| `.rar` | RAR Archive | ✔ | — | ✔ | — |

*Note: RAR extraction uses `github.com/nwaples/rardecode/v2` (pure Go). RAR export is unsupported as RAR is a proprietary write format; use `.tar.gz`, `.tgz`, or `.zip` for archive exports.*

## Full project import

```text
tk p import <file> [--name "Optional New Name"]
tk import <file> [--name "Optional New Name"]
tk p import - --format json|yaml|sql
```

Imports a complete project hierarchy (Project metadata, Categories, Tasks, Subtasks, Sprints) from a file or stdin (`-`).

Supported formats:
- **JSON**: Full `ExecutionTree` or `{ "project": ..., "categories": [...], "tasks": [...] }`.
- **YAML**: Full YAML `ExecutionTree`.
- **XLSX**: Multi-sheet workbook (`Project`, `Categories`, `Tasks`, `Subtasks`, `Sprints`).
- **CSV / TSV Archives**: `.zip`, `.tar.gz`, `.tgz`, `.tar`, `.rar` containing `project.csv`/`.tsv`, `categories.csv`/`.tsv`, `tasks.csv`/`.tsv`, `subtasks.csv`/`.tsv`, `sprints.csv`/`.tsv`.
- **SQL**: `INSERT INTO` statements for projects, categories, tasks, subtasks, and sprints.

Example output:
```text
Imported Project P-10556: "My Project Name"
  Categories: 3
  Tasks: 15
  Subtasks: 8
  Sprints: 12
```

## Batch task import

```text
tk t batch <project-id> <file> [--format csv|tsv|xlsx|json|yaml|yml|sql|zip|tar|tar.gz|tgz|rar|gz] [--check-only]
tk t batch <project-id> - --format csv|tsv|xlsx|json|yaml|yml|sql|zip|tar|tar.gz|tgz|rar
```

Reads legacy task-only files or a versioned nested execution batch. The exact accepted extensions, record schemas, archive rules, and validation sequence are specified in [Batch import format](batch-import-format.md). The CLI parses and validates format first, runs a no-write Core smoke test, and only applies a successful batch; failed checks never call an apply route. Legacy CSV/TSV/XLSX/SQL/archive formats remain task-only. Versioned nested batches use JSON or YAML and can create Tasks, Subtasks, and Sprints under an existing Project.

### Versioned nested execution batches

Use `timekeeper-execution-batch/v1` when a batch needs nested objects. The Project ID is supplied on the command line; `category_id` and extension owners must be existing IDs in that Project. Only `tasks` and `extensions` create objects. The optional `planning` section stores context and never creates or updates TimeKeeper objects.

```json
{
  "version": "timekeeper-execution-batch/v1",
  "tasks": [
    {
      "category_id": "C-10001",
      "name": "Build a feature",
      "goal": "Deliver one independently testable outcome",
      "estimated_duration_seconds": 3600,
      "subtasks": [
        {
          "name": "Implement and verify",
          "estimated_duration_seconds": 1800,
          "sprints": [
            { "name": "Implement the API", "estimated_duration_seconds": 900, "buffer_pct": 0 },
            { "name": "Add API tests", "estimated_duration_seconds": 900, "buffer_pct": 0 }
          ]
        }
      ]
    }
  ],
  "extensions": [
    { "task_id": "T-12345", "sprints": [{ "name": "Add follow-up coverage", "estimated_duration_seconds": 600 }] },
    { "subtask_id": "ST-12346", "sprints": [{ "name": "Verify edge cases", "estimated_duration_seconds": 600 }] }
  ],
  "planning": {
    "executable_task_metadata": [{ "task_index": 0, "key": "feature", "phase": 1, "acceptance": ["API and tests pass"] }],
    "deferred_items": [{ "key": "optional_provider", "condition": "Only after a provider contract is approved." }],
    "release_gates": [],
    "application_constraints": ["Do not deploy as part of this batch."]
  }
}
```

The equivalent YAML shape is:

```yaml
version: timekeeper-execution-batch/v1
tasks:
  - category_id: C-10001
    name: Build a feature
    goal: Deliver one independently testable outcome
    estimated_duration_seconds: 3600
    subtasks:
      - name: Implement and verify
        estimated_duration_seconds: 1800
        sprints:
          - name: Implement the API
            estimated_duration_seconds: 900
            buffer_pct: 0
          - name: Add API tests
            estimated_duration_seconds: 900
            buffer_pct: 0
extensions:
  - task_id: T-12345
    sprints:
      - name: Add follow-up coverage
        estimated_duration_seconds: 600
  - subtask_id: ST-12346
    sprints:
      - name: Verify edge cases
        estimated_duration_seconds: 600
planning:
  executable_task_metadata:
    - task_index: 0
      key: feature
      phase: 1
      acceptance: [API and tests pass]
  deferred_items:
    - key: optional_provider
      condition: Only after a provider contract is approved.
  release_gates: []
  application_constraints:
    - Do not deploy as part of this batch.
```

The importer rejects unknown keys, duplicate YAML mapping keys, extra documents, missing/unsupported versions for nested shapes, and malformed values rather than silently dropping them. A Task or Subtask may have at most four direct Sprints (including existing terminal Sprints); one batch may add at most 100 Tasks and 500 total executable objects. Sprint estimates must be positive seconds and `buffer_pct` must be a whole number from 0 through 100. Planning metadata does not count toward those limits.

Run the no-write smoke test by itself, or let the normal command smoke-test and then apply:

```text
tk t batch P-10053 docs/feature-batch.json --check-only
tk t batch P-10053 docs/feature-batch.json
```

A successful check-only run prints a checkmark and counts, then explicitly reports that no objects were created:

```text
✓ Smoke test passed: Tasks=1 Subtasks=1 Sprints=2 Total=4
check-only requested; no objects were created.
```

For a failed smoke test, the command exits nonzero, prints each safely discoverable issue with its object path, field, stable code, explanation, and direct repair suggestion, and does not print the pass checkmark or call apply:

```text
✗ Smoke test failed with 1 issue(s):
- tasks[0].category_id [foreign_category] Category belongs to a different Project
  Fix: Choose a Category ID from the target Project.
```

The main API validation response contains `valid`, executable `counts`, and an `issues` array. Each issue has `path`, `field`, `code`, `message`, and `suggestion`. Local syntax/schema failures include the offending path/location and correction guidance. At apply time Core repeats database-dependent checks inside the same SQLite transaction that creates every object; an apply-time validation rejection is returned as structured feedback and writes nothing. Successful nested apply returns every new Task, Subtask, Sprint ID and item address.

If the connection is lost during apply, the write outcome may be unknown. Do not blindly retry: inspect the target Project tree first and reconcile any created IDs. The CLI deliberately never retries an ambiguous batch write.

### Column mapping (CSV, TSV, XLSX)

| Column | Required | Type | Notes |
|---|---|---|---|
| `name` | ✔ | string | Task name |
| `category_id` | | string | e.g. `C-10001`; empty uses default |
| `description` | | string | Task description |
| `goal` | | string | Task goal |
| `priority` | | string | `Normal`, `High`, `Low` |
| `estimated_duration_seconds` | | integer | Duration in seconds |
| `estimate` | | duration | Human-friendly, e.g. `30m`, `2h`, `1h30m` |
| `auto_close` | | boolean | `true`/`false`/`yes`/`no`/`1`/`0` |

Use either `estimate` or `estimated_duration_seconds`, not both. Export-only metadata columns such as `task_id`, `project_id`, `item_address`, `status`, `completion_pct`, and timestamps are accepted but ignored. Unknown and duplicate columns are rejected; see [Batch import format](batch-import-format.md) for the full contract.

Example CSV:

```csv
name,category_id,estimate,priority,description
API persistence,C-10001,4h,High,Implement database layer
Migration script,C-10001,30m,Normal,Schema migration
```

### SQL format

```sql
-- Task import
INSERT INTO tasks (name, category_id, estimated_duration_seconds, priority) VALUES
('API persistence', 'C-10001', 14400, 'High'),
('Migration script', 'C-10001', 1800, 'Normal');
```

## Task export

```text
tk t export <project-id> <file> [--status S] [--category C]
tk t export <project-id> - --format csv|tsv|json|yaml|sql|xlsx|zip|tar.gz
```

Exports tasks via `GET /api/v1/projects/{projectID}/tasks` to any supported format. Estimates are formatted into human-friendly duration strings (`1h30m`). Filters: `--status Open`, `--category C-10001`.

## Full project export

```text
tk export <project-id> <file>
tk export <project-id>                  # JSON to stdout (existing behavior)
```

Exports the complete project hierarchy:
- **XLSX**: Multi-sheet workbook (`Project`, `Categories`, `Tasks`, `Subtasks`, `Sprints`).
- **ZIP / TAR.GZ**: Archive containing `project.csv`, `categories.csv`, `tasks.csv`, `subtasks.csv`, `sprints.csv` (or `.tsv`).
- **SQL**: `INSERT INTO` statements per entity type.
- **JSON / YAML**: Full `ExecutionTree`.
