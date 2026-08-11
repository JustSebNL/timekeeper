# Time Keeper API

Base URL: `http://127.0.0.1:1618/api/v1`

Time Keeper is a local, SQLite-authoritative HTTP/JSON API. It accepts only explicit numeric loopback listeners (`127.0.0.1` or `::1`). It has no authentication and must not be exposed beyond the local user boundary.

Every `POST` mutation requires `Content-Type: application/json`. Unknown request fields and non-JSON media types are rejected. Health remains exactly:

```json
{"status":"ok"}
```

## Implemented endpoints

```text
GET  /health
GET  /api/v1/projects
POST /api/v1/projects
POST /api/v1/projects/{projectID}/metadata
POST /api/v1/projects/{projectID}/status
GET  /api/v1/projects/{projectID}/execution-tree
GET  /api/v1/projects/{projectID}/operational-summary
GET  /api/v1/projects/{projectID}/events
GET  /api/v1/projects/{projectID}/notes
POST /api/v1/projects/{projectID}/notes
GET  /api/v1/projects/{projectID}/export
GET  /api/v1/projects/{projectID}/categories
POST /api/v1/projects/{projectID}/categories
  Optional body field: parent_category_id (a Category in the same Project).
POST /api/v1/categories/{categoryID}/metadata
POST /api/v1/categories/{categoryID}/status
GET  /api/v1/projects/{projectID}/tasks
POST /api/v1/projects/{projectID}/tasks
POST /api/v1/tasks/{taskID}/metadata
POST /api/v1/tasks/{taskID}/status
GET  /api/v1/tasks/{taskID}/subtasks
POST /api/v1/tasks/{taskID}/subtasks
GET  /api/v1/tasks/{taskID}/sprints
POST /api/v1/tasks/{taskID}/sprints
POST /api/v1/subtasks/{subtaskID}/status
GET  /api/v1/subtasks/{subtaskID}/sprints
POST /api/v1/subtasks/{subtaskID}/sprints
GET  /api/v1/sprints/{sprintID}/time-entries
GET  /api/v1/sprints/{sprintID}/extensions
POST /api/v1/sprints/{sprintID}/extensions
POST /api/v1/sprints/{sprintID}/start
POST /api/v1/sprints/{sprintID}/hold
POST /api/v1/sprints/{sprintID}/resume
POST /api/v1/sprints/{sprintID}/complete
GET  /api/v1/llm-pipelines
POST /api/v1/llm-pipelines
GET  /api/v1/projects/{projectID}/planning-drafts
POST /api/v1/projects/{projectID}/planning-drafts
POST /api/v1/projects/{projectID}/planning-drafts/generate
POST /api/v1/projects/{projectID}/planning-drafts/{draftID}/apply
```

## Project context

A Project's durable goal and description can be updated without changing its identity or workflow status:

```http
POST /api/v1/projects/P-10000/metadata
Content-Type: application/json

{"goal":"Ship a local tracker","description":"Durable execution context."}
```

Goal is bounded to 1,000 characters and description to 10,000. A material change atomically appends immutable `project_metadata_updated` activity; an identical update does not add event noise.

## Status workflows

Projects and Tasks accept these explicit values:

```text
Open | On Hold | Completed | Cancelled
```

Example:

```http
POST /api/v1/projects/P-10000/status
Content-Type: application/json

{"status":"On Hold"}
```

Non-no-op changes append immutable Project activity. Sprint status follows its separate lifecycle:

```text
Open → Active → On Hold → Active → Completed
```

Illegal Sprint actions return HTTP `409` with `invalid_transition` and the allowed actions.

## Sprint extensions

A Sprint extension is immutable evidence of justified additional planned time. It does not overwrite the original estimate or an existing extension.

```http
POST /api/v1/sprints/SP-10004/extensions
Content-Type: application/json

{"duration_seconds":600,"reason":"Migration uncovered an additional compatibility case."}
```

`duration_seconds` must be greater than zero and no more than ten years in seconds. `reason` is required after trimming and is bounded to 10,000 characters. The endpoint returns HTTP `201` and the created extension record. `GET /api/v1/sprints/{sprintID}/extensions` returns the immutable extension history in chronological order as:

```json
{"items":[{"extension_id":2,"sprint_id":"SP-10004","duration_seconds":600,"reason":"Migration uncovered an additional compatibility case.","created_at":"2026-08-11T00:00:00Z"}]}
```

## Local LLM planning (review-only)

A local pipeline is configuration for either Ollama or an OpenAI-compatible local server (such as vLLM or llama.cpp). Only plain HTTP numeric-loopback base URLs are accepted; remote hosts, `localhost`, HTTPS, paths, credentials, query strings, and fragments are rejected.

```http
POST /api/v1/llm-pipelines
Content-Type: application/json

{
  "name":"Planner",
  "provider":"ollama",
  "base_url":"http://127.0.0.1:11434",
  "model":"qwen3:4b"
}
```

Generate a draft for a Project:

```http
POST /api/v1/projects/P-10000/planning-drafts/generate
Content-Type: application/json

{"pipeline_id":1}
```

Generation passes bounded authoritative Project context to the selected local model. Model output must strictly validate as `timekeeper-planning-draft/v1`; it is stored as an immutable `Review` artifact with its exact raw JSON.

Apply requires a separate, deliberate request:

```http
POST /api/v1/projects/P-10000/planning-drafts/9/apply
Content-Type: application/json

{}
```

Apply revalidates the persisted artifact and atomically materializes its Categories, Tasks, Subtasks, and Sprints. A draft is then marked `Applied` and cannot be applied again. No model response directly mutates hierarchy.

## Project export and SQLite backup

`GET /api/v1/projects/{projectID}/export` is a versioned Project-scoped JSON export. It is not a complete database recovery snapshot.

Use server backup mode for a recoverable SQLite snapshot:

```text
timekeeper -db timekeeper.db -backup-to timekeeper-backup.db
```

The destination must not exist. Backups use `VACUUM INTO`, request owner-only mode on Unix-like filesystems, and remain unencrypted.
