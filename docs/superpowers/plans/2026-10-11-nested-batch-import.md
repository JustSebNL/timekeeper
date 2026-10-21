# Nested Execution Batch Import Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a versioned, smoke-tested batch workflow that reports actionable repairs and atomically creates Tasks, Subtasks, and Sprints in an existing Project while preserving legacy task-only imports.

**Architecture:** The CLI strictly parses versioned JSON/YAML into model inputs and preserves existing CSV/TSV/XLSX/SQL/archive parsing. Core exposes read-only validation endpoints for both legacy task-only and nested batches, plus one atomic nested apply endpoint; transaction-aware Store helpers recheck database-dependent rules before writes. A failed validation prints all safely discoverable structured issues and does not call an apply route.

**Tech Stack:** Go, `database/sql` with SQLite transactions, `net/http`, `encoding/json`, `gopkg.in/yaml.v3`, existing CLI/API/Store tests.

**Spec:** `docs/superpowers/specs/2026-10-11-nested-batch-import-design.md`

## Global Constraints

- The canonical nested document version is `timekeeper-execution-batch/v1`.
- Legacy task-only batch formats remain supported and keep their current apply endpoint semantics.
- Smoke-test validation performs no inserts, counter allocation, or Project events.
- Apply-time checks repeat inside the same SQLite transaction that creates the objects.
- Every invalid issue includes `path`, `field`, `code`, `message`, and actionable `suggestion` fields.
- The importer creates at most 100 new Tasks and 500 total Tasks + Subtasks + Sprints per batch.
- A Task or Subtask owns at most four direct Sprints, including terminal Sprints.
- Project/category/parent ownership and Open-state enforcement remain Core responsibilities.
- No partial-write mode, implicit apply retry, existing-object update, delete, or workflow-state import is allowed.
- Deferred and conditional migration proposals remain planning metadata and are not executable objects.

## Review Focus

- Unknown keys or a missing/unsupported version must fail with the exact document path and a schema/version correction.
- YAML scalar coercion, malformed YAML, and duplicate keys must not silently change values; malformed syntax reports a line/column or nearest useful location plus a repair hint.
- Two extensions that target the same existing owner must not bypass four-Sprint capacity through validation or apply; name the repeated owner path and explain how to split/remove work.
- A category, Task, or Subtask from another Project or in a closed state must produce safe actionable guidance without leaking SQL or arbitrary payload contents.
- A timeout after apply must not trigger automatic retry; CLI guidance must say to inspect the Project tree before trying again.

---

### Task 1: Define the versioned batch model and strict parser

**Files:**
- Create: `internal/model/execution_batch.go`
- Create: `internal/cli/execution_batch_import.go`
- Test: `internal/cli/execution_batch_import_test.go`
- Modify: `internal/cli/task_import.go`
- Test: `internal/cli/task_import_test.go`

**Interfaces:**
- Produce `model.ExecutionBatchInput`, `model.ExecutionBatchTask`, `model.ExecutionBatchSubtask`, `model.ExecutionBatchExtension`, `model.ExecutionBatchPlanning`, `model.ValidationIssue`, `model.BatchCounts`, and `model.BatchValidationResult`. `ExecutionBatchPlanning` allows only `executable_task_metadata`, `deferred_items`, `release_gates`, and `application_constraints`; none of these records are converted into writes.
- Produce `ParseExecutionBatch(filePath, format string) (model.ExecutionBatchInput, []model.CreateTaskInput, error)`. A non-empty `Version` selects the versioned nested contract; otherwise preserve `ImportTasks` legacy parsing. The legacy result is the parsed task slice and the nested result is the execution-batch input.
- Produce `model.ValidateExecutionBatchDocument(input model.ExecutionBatchInput) model.BatchValidationResult` for version, shape, field values, duplicate extension owners, object counts, and useful repair messages that do not require database reads. `cli.ValidateExecutionBatchDocument` is a thin compatibility wrapper; Store and CLI share the model implementation.
- Each issue has the stable JSON properties `path`, `field`, `code`, `message`, and `suggestion`; issues are emitted in document order.

- [x] **Step 1: Write failing parser and structural-validation tests** for a valid nested JSON document, the same YAML document, accepted non-executable planning metadata, unknown keys at executable and planning levels, missing/unsupported version, blank names, invalid estimates, invalid Sprint buffer, empty batch, duplicate existing-owner targets, limits, and multiple independent errors. Assert each issue's exact path/field/code and non-empty suggestion; assert deferred planning records add zero executable counts.
- [x] **Step 2: Run the new focused tests** with `go test ./internal/cli -run 'Test(ParseExecutionBatch|ValidateExecutionBatchDocument)' -count=1`. Expected: compile/test failure because the parser and model contract do not exist.
- [x] **Step 3: Add the model structs and strict JSON/YAML parser.** Decode versioned JSON with `json.Decoder.DisallowUnknownFields`; parse YAML while preserving duplicate-key diagnostics, convert supported YAML values to JSON, and run the same path-aware known-key scanner for both formats. Reject trailing documents/data. Do not change existing legacy importer callers or formats.
- [x] **Step 4: Implement structural validation** with stable ordered issues and bounded counts; do not include submitted arbitrary values in messages. For each invalid field, provide a direct correction (for example, a positive seconds value or a valid whole-number buffer from 0 to 100).
- [x] **Step 5: Run the focused tests** and confirm JSON/YAML pass, every invalid input reports repair guidance, and legacy importer tests remain green.

### Task 2: Add read-only and transactional Store batch operations

**Files:**
- Create: `internal/store/execution_batch.go`
- Create: `internal/store/execution_batch_test.go`
- Modify: `internal/store/store.go`
- Test: `internal/store/task_batch_test.go`
- Test: `internal/store/sprint_invariants_test.go`

**Interfaces:**
- Produce `(*Store).ValidateTasksBatch(ctx context.Context, projectID string, inputs []model.CreateTaskInput) (model.BatchValidationResult, error)`.
- Produce `(*Store).ValidateExecutionBatch(ctx context.Context, projectID string, input model.ExecutionBatchInput) (model.BatchValidationResult, error)`.
- Produce `(*Store).CreateExecutionBatch(ctx context.Context, projectID string, input model.ExecutionBatchInput) (model.ExecutionBatchResult, error)`.
- Introduce transaction-aware task/subtask/sprint insertion helpers accepting `*sql.Tx`, `context.Context`, parent IDs, and the existing `model.Create*Input` types. Public single-object create methods retain their behavior and delegate to these helpers where practical.
- Database-dependent validation checks project/category/owner existence and ownership, parent Open states, and persisted plus requested direct Sprint counts. `CreateExecutionBatch` returns IDs and addresses for every created object only after commit succeeds.
- Database read failures are returned as Go errors for safe HTTP 5xx mapping; they are not mislabeled as user-correctable batch issues.

- [x] **Step 1: Write failing Store validation tests** proving validation leaves Tasks/events/global item-number allocation unchanged; a foreign Category, closed extension owner, or fifth direct Sprint returns a repair issue; and legacy Task batch validation reports missing/foreign Categories.
- [x] **Step 2: Run `go test ./internal/store -run 'Test(Validate.*Batch|CreateExecutionBatch)' -count=1`.** Expected: fail because the Store validation methods do not exist.
- [x] **Step 3: Implement read-only validators** using SELECT-only transactions. Reuse the model-level structural validator, merge database-backed issues in stable path order, and provide actionable owner/Category/capacity guidance.
- [x] **Step 4: Write failing atomic-create tests** proving valid nested creation returns every ID/address and a late insert error leaves Tasks, Subtasks, Sprints, counters, and Project events unchanged; focused test first failed because `CreateExecutionBatch` was absent.
- [x] **Step 5: Implement transaction-aware insert helpers and atomic create.** Begin one transaction, repeat database-dependent validation against that transaction, allocate IDs only after validation succeeds, create parents before children, and roll back on any insert or commit failure.
- [x] **Step 6: Run the focused Store tests** and confirm unchanged-state validation, returned parent/child links, extension behavior, and late-failure rollback against the real SQLite test Store.

### Task 3: Expose validation and atomic apply through Core API

**Files:**
- Modify: `internal/api/server.go`
- Create: `internal/api/execution_batch_test.go`
- Test: `internal/api/server_test.go`

**Interfaces:**
- Add `POST /api/v1/projects/{projectID}/tasks/batch/validate`, accepting the legacy `{ "tasks": [...] }` payload and returning `model.BatchValidationResult` without writes.
- Add `POST /api/v1/projects/{projectID}/execution-batches/validate`, accepting strict versioned nested JSON and returning `model.BatchValidationResult` without writes.
- Add `POST /api/v1/projects/{projectID}/execution-batches`, accepting strict versioned nested JSON and returning `model.ExecutionBatchResult` only on atomic success.
- Keep existing `POST /api/v1/projects/{projectID}/tasks/batch` route/status/response behavior for legacy apply.

- [x] **Step 1: Write API tests** for validation response shape, multiple actionable issues, no changes to hierarchy/events on validation, strict unknown-field rejection, nested apply IDs, legacy task validation, and apply failure leaving the tree unchanged.
- [x] **Step 2: Run `go test ./internal/api -run 'TestExecutionBatch' -count=1`.** It first failed because routes were not registered.
- [x] **Step 3: Register the three routes and handlers.** Limit request bodies to 1 MiB, reject trailing JSON values and unknown nested fields, return structured issues for user-correctable validation failures, and map internal database failures to existing safe API error conventions.
- [x] **Step 4: Run focused API tests** and `go test ./internal/api -count=1`; confirm the legacy apply route remains unchanged.

### Task 4: Gate CLI imports on a successful smoke test

**Files:**
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/task_import.go`
- Create: `internal/cli/execution_batch_cli.go`
- Test: `internal/cli/execution_batch_cli_test.go`
- Test: `internal/cli/task_import_test.go`

**Interfaces:**
- Extend `taskBatch(projectID string, args []string, out, errOut io.Writer, baseURL string) int` to parse `--check-only` without changing file/format argument handling.
- Produce a formatter that renders `✓ Smoke test passed` only after a valid response and renders `✗ Smoke test failed; nothing was written (0 objects committed)` followed by each issue path, field, reason, and suggestion on failure.
- Legacy inputs call `/tasks/batch/validate` first, then `/tasks/batch` only on PASS. Versioned inputs call `/execution-batches/validate`, then `/execution-batches` only on PASS. `--check-only` returns after validation for either format.

- [x] **Step 1: Write failing CLI tests** with a local HTTP test server that records requests. Cover legacy and nested PASS, several simultaneous issues, no apply after failure, `--check-only` no apply, checkmark only on PASS, server validation errors, malformed local syntax with location/hint, and ambiguous apply timeout guidance with no automatic retry.
- [x] **Step 2: Run the focused CLI tests.** They first failed: existing code sent one direct write, ignored smoke validation, and treated `--check-only` as a filename.
- [x] **Step 3: Implement parse-route selection and validation-first flow.** Print issue arrays deterministically; never print submitted values or secrets. On apply transport error, explain that the outcome is unknown and instruct the user to inspect the Project tree before retrying.
- [x] **Step 4: Run focused CLI tests** and `go test ./internal/cli -count=1`; verify validation-before-apply ordering and exact no-apply behavior from observed server requests.

### Task 5: Document future batch files and migrate the Web UI manifest

**Files:**
- Modify: `docs/import-export.md`
- Modify: `docs/Extended-Web-UI-Migration-Objects.json`
- Modify: `docs/superpowers/specs/2026-10-11-nested-batch-import-design.md`
- Create: `internal/cli/testdata/execution-batch-valid.json`
- Create: `internal/cli/testdata/execution-batch-invalid.yaml`
- Test: `internal/cli/execution_batch_import_test.go`

**Interfaces:**
- Document JSON/YAML version `timekeeper-execution-batch/v1`, every supported field, examples, limits, smoke-test checkmark gate, `--check-only`, actionable issue shape, legacy formats, atomicity, and timeout/retry procedure.
- Convert only eligible migration work to executable `tasks`/`extensions`; retain phase, acceptance, release-gate, conditional, and deferred details under explicit planning metadata that the strict importer does not interpret as create operations.
- Add testdata files exercised by parser/CLI tests; ensure the committed migration manifest parses through the public import path and deferred records cannot be counted as executable work.

- [x] **Step 1: Add the canonical parser/artifact regression** asserting the JSON migration artifact contains the supported version and only declared executable objects contribute to create counts.
- [x] **Step 2: Inspect the existing manifest against the strict contract**; it used the unsupported `timekeeper-object-batch/v1` shape and mixed reuse/conditional planning data with proposed executable objects.
- [x] **Step 3: Convert the migration artifact** without dropping its planning details: conditional work is deferred, release gates remain planning metadata, and executable records use only documented v1 keys. Repeated keys (`registration_oidc`, `calendar`) remain one Task each with both phases in metadata.
- [x] **Step 4: Update `docs/import-export.md`** with a complete JSON example, YAML compatibility, passing/failing CLI output, error-field definitions, and safe-retry advice.
- [x] **Step 5: Run parser/artifact tests** and inspect the resulting JSON with the application parser rather than relying on visual validity alone.

### Task 6: Full compatibility and safety verification

**Files:**
- Test: `internal/cli/execution_batch_import_test.go`
- Test: `internal/cli/execution_batch_cli_test.go`
- Test: `internal/store/execution_batch_test.go`
- Test: `internal/api/execution_batch_test.go`
- Modify only any in-scope test or implementation file needed to resolve a reproduced failure.

**Interfaces:**
- No new production API. Verify the model, Store, API, CLI, legacy task importer, and migration artifact as one end-to-end contract.

- [x] **Step 1: Run focused package tests** with `go test ./internal/model ./internal/store ./internal/api ./internal/cli -count=1`; all four packages passed after the end-to-end integration regression was added.
- [x] **Step 2: Run the full Go suite** with `go test ./...`; the checkout-wide run stalled in `internal/cache`. Reproduced the specific unrelated failures with `go test ./internal/cache -count=1 -timeout=30s`: `TestDelete` fails, then `TestTTL` panics in `GetWithTTL` and hangs in deferred `Close`. Existing checkpoint M-1178 noted this; a separate open defect T-11566/SP-11567 was logged. All other packages passed with `go test -count=1`.
- [x] **Step 3: Run `git diff --check`** and inspect only plan-scoped paths; preserve all unrelated dirty files. No trailing whitespace was found in untracked deliverables; Go files are gofmt'd.
- [x] **Step 4: Use a disposable Store and real Core API test server** to demonstrate smoke PASS/check-only without writes and subsequent atomic nested apply. Separate regressions cover failures, repair feedback, and no apply on failed smoke. The migration artifact was parsed/validated only; do not apply it to the live Project or database.
- [x] **Step 5: Record exact test evidence and remaining limitations in TimeKeeper**. The source change is not installed or deployed. Checkout-wide suite limitation is separately tracked; no production or live-project mutations occurred.
