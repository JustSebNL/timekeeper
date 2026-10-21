# Task batch import format

This document is the contract for `tk t batch <project-id> <file>`. It defines the accepted file formats, the task fields they map to, and the validation gate that must pass before Core receives a write request.

For full-Project import/export (`tk p import` / `tk export`), see [Import and export](import-export.md). Those commands use a different, full-hierarchy contract.

## Safety rule

A batch is not submitted to an apply route until both its file format and its contents pass validation:

1. The CLI identifies the format and parses the entire file. Unsupported syntax, fields, headers, types, duplicate keys, or invalid values fail locally.
2. The CLI checks structural limits and required Task fields.
3. The CLI sends the parsed payload to the Project-scoped, read-only validation endpoint. Core verifies that the target Project exists and is Open; every new Task's Category exists, belongs to that Project, and is Open; every extension Task/Subtask owner exists in that Project with an Open parent chain; and existing Sprint capacity and database-dependent constraints still allow the complete batch. The Core validation request allocates no IDs, rows, events, or other durable Core objects.
4. For an apply-mode command, the CLI first persists a local outbox item and stable `Idempotency-Key`. Only a successful Core validation response permits the apply request. `--check-only` stops after validation and never creates a local offer or applies.
5. Core repeats state-dependent validation inside the write transaction. If it fails, the whole batch rolls back; no partial batch is committed.

A parse or local validation error is a hard stop and creates no outbox item. A Core validation rejection is retained in the local outbox with repair findings and never calls apply. If the connection fails after apply begins, the local receipt and stable key remain retryable; use `tk queue show <local-id>` or `tk queue flush` rather than submitting a fresh duplicate.

The local client outbox lives under `.timekeeper/registration-outbox`. `tk queue list` and `tk queue show` inspect local requests; `tk queue flush [--limit N]` sends pending requests FIFO and re-runs batch smoke validation before apply. The local outbox is not the Core registration queue: an offer appears in the Web UI only after Core accepts and persists it. Direct `tk t new`, `tk st new`, and `tk sp new` use the same outbox for offline/ambiguous delivery.

## Offline Core execution against a copy

The Core binary has an offline maintenance mode for environments where no HTTP service should be started. It always creates and operates on a new output copy; it never applies a batch to the source database:

```text
timekeeper -db <standalone-source.db> \
  -offline-batch-file <batch-file> \
  -offline-batch-project <project-id> \
  -offline-batch-copy-to <new-output.db>
```

The default is a Core smoke test only. Add `-offline-batch-apply` to apply, but only to the new output copy and only after the same Core preflight succeeds. The command starts no HTTP listener, runs the shared accepted-format parser, takes a byte-for-byte snapshot under the source's Core instance lock, and integrity-checks the hash-matched output copy. It then invokes the same Core API batch-validation and apply handlers in-process, preserving API behavior without opening a socket. Apply first commits a FIFO registration-queue offer; Core then atomically registers its Task/Subtask/Sprint rows and records the queue result. The queue receipt remains in the copied database for inspection. The source database is never opened through SQLite. It refuses an active Core lock, any SQLite `-wal`/`-shm`/`-journal` sidecar, a corrupt source copy, or an output path that already exists. It never overwrites or promotes a database. Use a verified standalone candidate as the source; a live database with sidecars is deliberately not a valid input.

The source hash is read before and after copying and after the batch operation. Keep the resulting output copy separate until its contents and database state have been reviewed; promoting it is a distinct, explicitly gated recovery action.

## Accepted task-batch formats

| Input | Accepted shape | Scope |
|---|---|---|
| `.json` | Legacy task list or versioned `timekeeper-execution-batch/v1` document | Flat Tasks or nested Tasks/Subtasks/Sprints |
| `.yaml`, `.yml` | Legacy task list or versioned `timekeeper-execution-batch/v1` document | Flat Tasks or nested Tasks/Subtasks/Sprints |
| `.csv` | Header row plus one Task per row | Flat Tasks only |
| `.tsv` | Tab-separated header row plus one Task per row | Flat Tasks only |
| `.xlsx` | First worksheet; first row is the header | Flat Tasks only |
| `.sql` | Restricted `INSERT INTO tasks (...) VALUES (...);` statements | Flat Tasks only; SQL is parsed, never executed |
| `.zip`, `.tar`, `.tar.gz`, `.tgz`, `.rar` | Archive containing supported task-data files | Flat Tasks only; contents are combined |
| Single-file `.gz` | A supported file compressed as `name.<inner-format>.gz`, such as `tasks.csv.gz` | Flat Tasks only |

Legacy `.xls` workbooks are not supported. Use `.xlsx`. A nested execution-batch document must be provided directly as JSON or YAML; nesting that document inside an archive or gzip is not supported.

Archive directories are ignored. Every regular file in an archive must be CSV, TSV, JSON, YAML/YML, or SQL; an unsupported member or an archive with no importable files rejects the whole archive. XLSX is accepted as a standalone batch file, not as an archive member. A bad member never results in a partial submission.

## Shared flat-Task record

CSV, TSV, XLSX, legacy JSON/YAML, SQL, and supported archive members normalize to the same Task import record.

| Field | Required | Type and meaning |
|---|---|---|
| `name` | Yes | Non-blank Task name. |
| `category_id` | No | Existing Category ID or name in the target Project. Blank/default values use the Project's default Category. |
| `description` | No | Task description string. |
| `goal` | No | Task goal string. |
| `priority` | No | String; use `Normal`, `High`, or `Low` for standard values. Blank follows the normal Task default. |
| `estimate` | No | Positive duration string, such as `30m`, `2h`, `1h30m`, or positive whole seconds. |
| `estimated_duration_seconds` | No | Non-negative integer seconds. Use this OR `estimate`, not both. |
| `auto_close` | No | Boolean. CSV/TSV/XLSX accept `true`, `false`, `yes`, `no`, `1`, or `0`; JSON/YAML use a boolean value; SQL accepts `0`/`1`, `TRUE`/`FALSE`, or a single-quoted boolean word. |

The Project ID is supplied to `tk t batch`; a row cannot redirect itself to another Project. Category ownership is checked by Core during preflight. A legacy flat batch must contain 1–100 Tasks; an empty batch is rejected.

Task exports may include these additional, read-only metadata fields: `task_id`, `project_id`, `item_address`, `status`, `completion_pct`, `reported_completion_pct`, `calculated_completion_pct`, `created_at`, and `updated_at`. They are accepted for round-trip files but ignored on import; IDs, status, timestamps, and completion state are assigned or managed by Core. Timestamps must use RFC3339; percentage fields must be finite numbers.

Unknown fields/columns, duplicate fields/headers, blank headers, malformed durations, invalid booleans, negative seconds, malformed timestamps, or rows with the wrong number of columns are rejected before the API validation request. Blank separator rows are ignored.

## CSV and TSV

The first row is the header. Header matching is case-insensitive and trims surrounding whitespace. `name` is required. Each nonblank row must have exactly the same number of fields as the header.

CSV example:

```csv
name,category_id,estimate,priority,auto_close,description
API persistence,C-10001,4h,High,true,Implement the database layer
Migration script,C-10001,30m,Normal,false,Add the schema migration
```

TSV uses the same columns and rules, separated by tabs:

```tsv
name	category_id	estimated_duration_seconds	priority
API persistence	C-10001	14400	High
Migration script	C-10001	1800	Normal
```

The exported metadata columns listed above may also appear. They do not overwrite existing IDs or workflow state.

## XLSX

Use `.xlsx` (not `.xls`). The importer reads the first worksheet and treats its first row as the header. Use the same column names and value rules as CSV. The exporter writes a worksheet named `Tasks`; other worksheets are not used. Spreadsheet libraries may omit trailing blank cells from a row, so those are treated as empty; nonblank cells beyond the header width are rejected.

## Legacy JSON and YAML

Legacy JSON accepts either a bare array or a `tasks` wrapper:

```json
{
  "tasks": [
    {
      "category_id": "C-10001",
      "name": "API persistence",
      "description": "Implement the database layer",
      "goal": "Persist task batches atomically",
      "priority": "High",
      "estimated_duration_seconds": 14400,
      "auto_close": true
    }
  ]
}
```

A bare array of those Task objects is also accepted. YAML uses the equivalent sequence or `tasks:` mapping. A legacy row may include the documented export-only metadata fields, but unknown keys and duplicate JSON/YAML keys are rejected. The file must contain exactly one document.

Legacy JSON/YAML creates Tasks only. To create nested objects, use the versioned contract below.

## Versioned nested JSON/YAML

Nested execution batches use the exact version string `timekeeper-execution-batch/v1` and may contain `tasks`, `extensions`, and optional `planning` metadata. The Project is still supplied on the command line. `tasks` create new Tasks and optional descendants; `extensions` add Subtasks/Sprints under existing open Task/Subtask IDs; `planning` is informational only and never creates objects. Unknown keys and unversioned nested shapes are rejected.

The complete JSON and YAML examples, field paths, validation issue format, and object limits are in [Import and export — Versioned nested execution batches](import-export.md#versioned-nested-execution-batches). Limits are at most 100 Tasks and 500 total created Tasks/Subtasks/Sprints per batch, with at most four direct Sprints per owner.

## SQL

The SQL format is a deliberately restricted data interchange syntax, not a SQL execution feature. It accepts only one or more statements of this form:

```sql
-- Text is single-quoted; escape an apostrophe by doubling it.
INSERT INTO tasks (
  task_id, category_id, name, description, goal, status, priority,
  estimated_duration_seconds, auto_close
) VALUES
  ('T-10001', 'C-10001', 'API persistence', 'Implement the database layer',
   'Persist task batches atomically', 'Open', 'High', 14400, 1),
  ('T-10002', 'C-10001', 'Migration script', '', '', 'Open', 'Normal', 1800, 0);
```

Only the `tasks` target and documented task-import columns are accepted. Each statement must end with a semicolon. Multiple rows per `VALUES` list and multiple task `INSERT` statements are supported. `--` and `/* ... */` comments are allowed. Text values must be single-quoted with apostrophes doubled; numeric fields must use unquoted numeric literals; optional values may use `NULL`. No DDL, `UPDATE`, `DELETE`, other table targets, or arbitrary SQL is executed. Export-only IDs/status/timestamps are ignored as described above.

## Validation command

```text
tk t batch P-10053 tasks.csv --check-only
tk t batch P-10053 nested-batch.json --check-only
```

A successful check prints a pass mark and counts, then states that no objects were created. A regular `tk t batch` runs the same check first and applies only after success. Any format, local, or Core validation failure exits before apply and prints repair guidance.

The API endpoints are `POST /api/v1/projects/{projectID}/tasks/batch/validate` for flat Task imports and `POST /api/v1/projects/{projectID}/execution-batches/validate` for versioned nested batches. Apply endpoints repeat database-dependent validation atomically to close the gap between preflight and write.

For the other import/export workflows, see [Import and export](import-export.md).