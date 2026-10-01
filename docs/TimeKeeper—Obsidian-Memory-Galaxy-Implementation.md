# TimeKeeper — Obsidian Memory Galaxy Implementation

## Goal

Add an **Obsidian Memory Galaxy** integration to TimeKeeper.

The Galaxy is a cross-project storage layer for:

- long-horizon memory
- reusable knowledge
- RAG sources
- skills
- harnesses
- patterns
- decisions
- lessons/failures
- research/reference material
- evidence-backed self-improvement across projects

TimeKeeper's existing **Trail of Thought** remains project-local working memory.

```text
Current Agent Context
        ↓
Trail of Thought
(project-local)
        ↓
Promote useful knowledge
        ↓
Obsidian Memory Galaxy
(cross-project / long-horizon)
        ↓
Search / RAG / retrieval
```

---

## Design status

This document is the implementation specification for the Memory Galaxy integration.
Trail of Thought remains the currently available project-local memory surface.

Obsidian is the canonical Memory Galaxy vault and editor. Backup providers may
mirror encrypted snapshots of that vault, but they are not alternate live
Galaxy stores and must never replace Obsidian as the canonical write target.

### Implementation progress

Phase 0/1 has started in TimeKeeper:

- `internal/knowledge` defines the provider-neutral contract, bounded candidate/query validation, and call deadlines.
- `internal/knowledge/obsidian` provides a descriptor-driven, numeric-loopback HTTP client with token-file authentication and typed errors.
- `knowledge_links` persists explicit promotion mappings with idempotent promotion keys, nullable pending Galaxy IDs, status transitions, and duplicate-ID protection.
- Focused tests cover the contract, endpoint validation, authenticated requests, migration, retries, and persistence.

The provider contract/client, Knowledge read routes, redaction-gated Trail promotion, durable `knowledge_links`, dashboard promotion control, and plugin settings/package contract are implemented and fixture-tested in source. The Obsidian repository source validates schema-v1 frontmatter and UTC timestamps, preserves topics/tags, recalculates canonical content hashes, serializes concurrent promotion retries through one repository instance, quarantines duplicate Galaxy IDs in a visible `Quarantine/` folder, and returns topic/tag metadata through Get/Search. These behaviors are fixture- and API-tested; the extension test suite passes. This does not establish a live Obsidian installation or smoke. That process smoke and release artifact remain separate at T-10321/SP-10322 and are On Hold while Obsidian is unavailable. Do not describe the Galaxy integration as fully available until the live checks pass.

## Non-goals

The first implementation must not become any of the following:

- a general TimeKeeper plugin marketplace or plugin/store framework
- a replacement for Trail of Thought
- a second project/task database
- an automatic exporter of every Trail entry
- a multi-user cloud knowledge service
- a vector database or document-ingestion platform
- a custom visual graph editor

Promotion is explicit and user-controlled. Keyword search and bounded Markdown
items are sufficient for the first release. RAG ingestion and semantic search
remain later adapters behind the same provider boundary.

---

# 1. Architecture

Do **not** build a general plugin/store system.

Add a small knowledge abstraction to TimeKeeper:

```text
TimeKeeper
    │
    ├── Existing project system
    ├── Trail of Thought
    │
    └── KnowledgeService
             │
             ▼
      Obsidian Provider
             │
             ▼
      Obsidian Plugin
             │
             ▼
       Memory Galaxy
```

The abstraction should allow another knowledge provider later, but only Obsidian needs implementing now.

---

# 2. TimeKeeper Remains Independent

TimeKeeper must work normally when Obsidian is:

- not installed
- closed
- disconnected
- Galaxy integration disabled

The Galaxy is an additional knowledge layer, not a TimeKeeper dependency.

---

# 3. Communication

Follow the pattern already used by the VS Code integration.

```text
TimeKeeper
    ↕
local HTTP/JSON
    ↕
Obsidian Memory Galaxy Plugin
```

Do not let the Obsidian plugin access TimeKeeper's SQLite database directly.

The Obsidian plugin should expose a local loopback API that TimeKeeper can call.

## 3.1 Local API trust boundary

Loopback binding is a network-location restriction, not authentication. Both
the TimeKeeper client and the Obsidian plugin API must:

- bind only to `127.0.0.1` (never `0.0.0.0` or an externally reachable address)
- require a per-install capability token for every non-health request
- store the token outside the Galaxy Markdown files and source repository
- reject cross-origin requests by default; CORS is not the authentication layer
- enforce request-body, query-length, result-count, and request-timeout limits
- return typed errors without exposing arbitrary filesystem paths or raw dumps

The unauthenticated health endpoint may expose only liveness, protocol version,
and a coarse availability state. It must not return the token, vault path, or
Galaxy content.

## 3.2 Endpoint discovery and lifecycle

The Obsidian plugin may use a configured or dynamically selected loopback port,
but TimeKeeper must not assume a hard-coded port. On startup/reconnect, the
plugin writes an authenticated endpoint descriptor containing:

- loopback address and selected port
- protocol version
- provider name and vault identity
- a token reference/handshake identifier, never the token in a Markdown item

TimeKeeper re-reads this descriptor when Obsidian restarts. A port collision,
missing vault, incompatible protocol, or failed bind is reported as a provider
state; it must not prevent normal TimeKeeper startup.

---

# 4. TimeKeeper Knowledge Service

Add something equivalent to:

```go
type KnowledgeProvider interface {
    Status(ctx context.Context) (ProviderStatus, error)

    Search(
        ctx context.Context,
        query KnowledgeQuery,
    ) ([]KnowledgeResult, error)

    Get(
        ctx context.Context,
        id string,
    ) (KnowledgeItem, error)

    Create(
        ctx context.Context,
        item KnowledgeCandidate,
        idempotencyKey string,
    ) (KnowledgeItem, error)
}

type ProviderStatus struct {
    Available       bool
    Provider        string
    ProtocolVersion string
    VaultID         string
    Capabilities    []string
    LastCheckedAt   time.Time
    Message         string
}
```

`Create` must be idempotent. A lost response after a successful file write must
not create a second Galaxy item when TimeKeeper retries. The idempotency key is
derived from the source identity and promotion revision, for example:

```text
trail:<trail_id>:promotion:<revision>
```

The provider returns the existing Galaxy item when that key was already
committed. Context deadlines are mandatory on every provider call.

Implement:

```text
ObsidianKnowledgeProvider
```

Do not put Obsidian-specific behaviour into the rest of TimeKeeper.

Recommended structure:

```text
internal/
    knowledge/
        service.go
        types.go

    knowledge/obsidian/
        provider.go
        client.go

    api/
        knowledge.go
```

Adapt this to existing package conventions if required.

---

# 5. Galaxy Item

Every Galaxy item needs a permanent ID independent of its filename.

Example:

```text
MG-01K53JX8VTYD4M8Q71XG9FH3AT
```

Recommended Markdown:

```yaml
---
galaxy_id: MG-01K53JX8VTYD4M8Q71XG9FH3AT
schema_version: 1

title: Capability restricted RPC
type: pattern

created_at: 2026-09-14T09:00:00Z
updated_at: 2026-09-14T09:00:00Z

origin:
  system: timekeeper
  project_id: P-10000
  task_id: T-10014
  trail_id: TR-10492

review_status: inbox
sync_status: created
sync_revision: 1
content_hash: sha256:...

topics:
  - extension-architecture
  - security
  - rpc
---

# Capability restricted RPC

Reusable knowledge goes here.
```

The Markdown body is the actual knowledge content.

## 5.1 Canonical Markdown schema

The frontmatter is a versioned contract, not advisory metadata. The initial
schema requires:

```yaml
galaxy_id: MG-...
schema_version: 1
title: ...
type: knowledge | decision | pattern | failure | insight | procedure | reference | rag-source | skill | harness | research
review_status: inbox | draft | verified | archived | rejected
sync_status: created | updated | conflict
sync_revision: 1
content_hash: sha256:...
created_at: 2026-09-14T09:00:00Z
updated_at: 2026-09-14T09:00:00Z
origin: ...
topics: []
tags: []
visibility: private | galaxy
```

Rules:

- `galaxy_id` is immutable and independent of the filename.
- Unknown frontmatter fields are rejected or preserved only through an explicit
  future schema version; silently dropping them is not allowed.
- `title`, topics, tags, and body content have bounded lengths.
- Timestamps are UTC RFC 3339 values.
- Markdown files are written atomically and duplicate IDs are quarantined rather
  than silently merged.
- `content_hash` covers the canonical body and normalized frontmatter fields so
  changes can be detected without comparing file paths.
- `visibility` is explicit. The first release supports `private` and `galaxy`;
  project-wide ACLs are outside the MVP.

`review_status` describes knowledge curation. `sync_status` describes provider
convergence. They must not be combined into one ambiguous status field.

---

# 6. Galaxy Item Types

Initially support:

```text
knowledge
decision
pattern
failure
insight
procedure
reference
rag-source
skill
harness
research
```

These are values of `type`, not separate storage systems.

---

# 7. Galaxy Structure

Suggested default:

```text
Memory Galaxy/
│
├── Inbox/
├── Knowledge/
├── Decisions/
├── Patterns/
├── Failures/
├── Procedures/
├── Research/
├── References/
├── RAG/
├── Skills/
├── Harnesses/
└── Archive/
```

Do not create folders per TimeKeeper project.

The Galaxy is cross-project.

---

# 8. Project Relationships

Do not permanently assign Galaxy items to every project they might be useful for.

Store only their origin where available:

```yaml
origin:
  project_id: P-10000
```

Future projects discover relevant Galaxy items through search.

The intended model is:

```text
Project
   │
   └── asks Galaxy for relevant knowledge
```

not:

```text
Galaxy item
   └── manually updated whenever a Project is created
```

Cross-project discoverability is not the same as authorization. Promotion is
always explicit. Sensitive material, credentials, cookies, signed URLs, raw
transcripts, and arbitrary tool-output dumps must not be promoted. The UI
should show a redaction and visibility preview before sending a Trail item to a
cross-project `galaxy` item.

---

# 8.1 Self-improvement loop

The Galaxy is the agent's long-term learning layer. It should let a later agent
or later project retrieve verified knowledge that originated elsewhere without
copying a project board or replaying an old conversation.

The intended loop is:

```text
Observe outcome in a Project
        ↓
Record evidence and lesson on the project Trail
        ↓
Explicitly promote a reusable candidate
        ↓
Review, verify, and classify in the Galaxy Inbox
        ↓
Retrieve before related future work
        ↓
Apply the lesson and record whether it helped
```

Self-improvement means changing future decisions, planning, retrieval, and
verification behavior from durable evidence. It does not mean silently
rewriting model weights, persona rules, credentials, or source code. Any such
change remains a separate reviewed engineering action.

A useful self-improvement item should state:

- the observed situation and source Project;
- the evidence that made the lesson trustworthy;
- the reusable rule, pattern, failure, skill, or harness guidance;
- the boundary where the lesson should not be applied;
- how a future agent can verify whether it helped.

`review_status: verified` is a curation claim, not a model-quality guarantee.
The agent should prefer verified Galaxy items, preserve provenance, and record
new contradictory evidence rather than silently overwriting the old lesson.

---

# 9. Trail of Thought Integration

Add a new action to Trail entries:

```text
Promote to Galaxy
```

Flow:

```text
Trail Item
    ↓
Promote to Galaxy
    ↓
Galaxy Candidate
    ↓
Memory Galaxy / Inbox
```

The original Trail item stays where it is.

Store a mapping so TimeKeeper knows that the Trail item has already been promoted.

Example table:

```text
knowledge_links

source_type
source_id
galaxy_id
promotion_key
promotion_status
last_error
created_at
updated_at
```

Example:

```text
trail | TR-10492 | MG-01K... | timestamp
```

This prevents accidental duplicate promotion.

## 9.1 Promotion state machine

Promotion is a durable state transition, not a fire-and-forget HTTP request:

```text
not_promoted
    ↓
pending
    ├── created
    ├── failed_retryable
    ├── failed_permanent
    └── conflict
```

The `knowledge_links` table must enforce uniqueness for:

```text
(source_type, source_id, promotion_key)
galaxy_id
```

If the request times out after the plugin may have committed the file,
TimeKeeper retries with the same idempotency key and reconciles by key. It must
not blindly create a new candidate. A failed promotion never invalidates or
deletes the original Trail item.

## 9.2 Ownership and synchronization

For the MVP:

```text
TimeKeeper owns:
- promotion requests
- source provenance
- knowledge_links
- provider configuration and status

Obsidian owns:
- Galaxy Markdown body
- review_status
- topics and tags
- file location
- Galaxy item persistence
```

MVP synchronization is intentionally one-way for writes:

- TimeKeeper → Obsidian: create a candidate
- Obsidian → TimeKeeper: search and get the current item
- TimeKeeper does not update Markdown directly

Future write-back must use optimistic concurrency with a revision, content
hash, or ETag/If-Match contract. Silent last-writer-wins behavior is forbidden.

---

# 10. Promotion Payload

TimeKeeper should send something similar to:

```json
{
  "title": "Capability restricted RPC",
  "content": "Reusable knowledge...",
  "type": "pattern",
  "visibility": "galaxy",
  "promotion_key": "trail:TR-10492:promotion:1",

  "origin": {
    "project_id": "P-10000",
    "task_id": "T-10014",
    "trail_id": "TR-10492"
  }
}
```

Obsidian creates the Markdown file and returns:

```json
{
  "id": "MG-01K53JX8VTYD4M8Q71XG9FH3AT",
  "review_status": "inbox",
  "sync_status": "created",
  "sync_revision": 1,
  "duplicate": false
}
```

The response must be safe to repeat: the same `promotion_key` returns the same
Galaxy ID with `duplicate: true` rather than creating another file.

---

# 11. Required Obsidian API

MVP needs only:

```text
GET  /health
GET  /api/v1/galaxy/status

POST /api/v1/galaxy/items
GET  /api/v1/galaxy/items/{id}

POST /api/v1/galaxy/search
```

Avoid adding a large API until actual functionality requires it.

All non-health routes require the capability token. Responses use one stable
error envelope:

```json
{
  "error": {
    "code": "provider_unavailable",
    "message": "Memory Galaxy is unavailable",
    "retryable": true
  }
}
```

The API must define maximum request sizes, maximum search limits, request
deadlines, and protocol-version negotiation before implementation begins.

---

# 12. TimeKeeper API

Expose Galaxy functionality through TimeKeeper so agents do not need to know about Obsidian.

Recommended:

```text
GET  /api/v1/knowledge/status

POST /api/v1/knowledge/search

GET  /api/v1/knowledge/{id}

POST /api/v1/trails/{trail_id}/promote
```

TimeKeeper remains the public-facing owner of these routes. Obsidian-specific
URLs, ports, token headers, and response shapes stay inside the provider
adapter.

---

# 13. Search

Start with normal local search.

Search across:

- title
- Markdown body
- type
- topics
- tags

Request:

```json
{
  "query": "persistent agent context",
  "limit": 8
}
```

Response:

```json
{
  "results": [
    {
      "id": "MG-...",
      "title": "SQLite context offloading",
      "type": "pattern",
      "summary": "...",
      "topics": ["sqlite", "context"]
    }
  ]
}
```

Keep results concise because one of the purposes of this system is reducing context-window usage.

Search contract:

- query length, result count, and response bytes are bounded
- the default result contains metadata and a short summary only
- full content requires `GET /api/v1/knowledge/{id}`
- ranking is deterministic for equal inputs
- pagination/cursors are added before result sets can grow beyond the bounded
  response budget
- a corrupt or unavailable index returns a typed provider error, not a silently
  empty result set

---

# 14. RAG

Design Galaxy storage so RAG can be added without redesigning it.

A RAG source should be represented as a Galaxy item:

```yaml
type: rag-source
```

Source files and derived indexes should remain separate.

These are three different concerns:

```text
Galaxy item   = human-curated reusable knowledge
RAG source    = original source metadata and provenance
Derived index = rebuildable keyword/vector retrieval artifact
```

A future `rag-source` item should carry bounded provenance such as:

```yaml
source_uri: ...
source_hash: sha256:...
source_kind: file | url | repository | paper
indexed_at: 2026-09-14T09:00:00Z
index_version: 1
```

The original source and derived index are never treated as the canonical
knowledge body. RAG ingestion remains outside the first Galaxy release.

```text
Original Source
      ↓
Galaxy RAG metadata
      ↓
Search/index
```

Do not require vector search for the first implementation.

Add semantic/vector retrieval later behind the Obsidian search implementation.

TimeKeeper should continue calling the same:

```text
KnowledgeService.Search()
```

regardless of whether Obsidian uses keyword search, FTS, LanceDB, or another local index.

---

# 15. Obsidian Plugin

Recommended structure:

```text
extensions/
    obsidian-memory-galaxy/
        manifest.json
        package.json

        src/
            main.ts

            api/
                server.ts

            galaxy/
                repository.ts
                schema.ts
                ids.ts

            search/
                search.ts

            ui/
                settings.ts
                inbox.ts
```

Main responsibilities:

```text
Receive TimeKeeper requests
Create/update Galaxy Markdown
Search Galaxy
Return Galaxy items
Manage candidate Inbox
```

---

# 16. Initial Obsidian UI

Keep the UI minimal.

### Settings

```text
Memory Galaxy

Galaxy folder:
Memory Galaxy/

TimeKeeper connection:
Connected

[Reconnect]
```

### Candidate Inbox

Incoming promoted Trail items should be visible for editing/classification.

Example:

```text
Capability restricted RPC

Origin:
TimeKeeper / P-10000 / T-10014

Type:
[Pattern]

[Verify]
```

Do not build a custom visual Galaxy yet.

Obsidian's existing graph/backlink features are sufficient initially.

---

# 17. TimeKeeper UI

Add only:

### Trail

```text
[Promote to Galaxy]
```

After promotion:

```text
✓ Galaxy
[Open in Obsidian]
```

### Settings/status

```text
Memory Galaxy
● Connected
```

### Knowledge search

A simple Galaxy search can be added either globally or inside a project.

---

# 18. Failure Handling

If Galaxy cannot be reached, TimeKeeper reports:

```text
Memory Galaxy unavailable
```

and preserves the provider/promotion state. It must not block or alter normal
TimeKeeper operation, and it must not retry indefinitely.

The minimum failure matrix is:

| Failure | TimeKeeper behavior | Obsidian behavior |
|---|---|---|
| Obsidian closed | show disconnected; retain promotion as pending | none |
| Port unavailable | bounded reconnect; preserve state | report bind failure |
| Timeout after Create | retry with the same promotion key | may already have committed; deduplicate |
| Duplicate promotion | return the existing Galaxy ID | do not create a second file |
| Malformed Markdown | return a typed schema error | quarantine the item |
| Invalid capability token | show authentication/provider failure | reject the request |
| Protocol mismatch | show compatibility error | expose supported protocol version |
| Vault moved or missing | show unavailable until reconfigured | report missing vault |
| Search index unavailable | return a typed search error | do not silently return empty results |

Every provider request has a deadline. Retryable failures use a bounded retry
policy or explicit user retry; permanent failures remain visible in the mapping.
A timeout must never be treated as proof that creation did not happen.

---

# 19. Implementation Order

## Phase 0 — Contracts and threat model

Before production wiring, freeze:

- provider protocol version and capability negotiation
- loopback bind and endpoint-discovery rules
- capability-token storage and request authentication
- canonical Markdown schema and size limits
- promotion idempotency key and `knowledge_links` uniqueness
- failure/error envelope and retry policy
- mock-provider and provider-contract test fixtures

Result:

```text
The boundaries are testable before either integration is trusted.
```

---

## Phase 1 — TimeKeeper Knowledge Layer

Implement:

- `KnowledgeService`
- provider interface
- Obsidian provider/client
- configuration
- provider status
- provider deadlines and bounded retries
- `knowledge_links` persistence/migration
- tests

Result:

```text
TimeKeeper can communicate with a mock Galaxy and retain truthful promotion state.
```

---

## Phase 2 — Obsidian Plugin

Implement:

- plugin skeleton
- Galaxy folder
- authenticated local API
- endpoint descriptor/discovery
- Galaxy repository
- stable IDs and idempotency
- Markdown schema validation
- atomic file writes and duplicate quarantine
- health/status endpoints

Result:

```text
TimeKeeper ↔ Obsidian connection works without weakening the local security boundary.
```

---

## Phase 3 — Trail Promotion

Implement:

- Promote to Galaxy action
- redaction and visibility preview
- candidate creation
- provenance
- Trail → Galaxy mapping
- duplicate prevention
- retry/reconciliation after lost acknowledgements

Result:

```text
Trail item → Obsidian Inbox with durable, recoverable promotion state.
```

---

## Phase 4 — Search

Implement:

- Galaxy search
- bounded summaries
- deterministic result ordering
- TimeKeeper Knowledge API
- search UI
- unavailable/corrupt-index error handling

Result:

```text
A different TimeKeeper project can retrieve knowledge created
from another project without copying or relinking the item.
```

---

## Phase 5 — RAG

Add:

- RAG source type
- source provenance and content hashes
- source indexing
- reindex capability
- richer retrieval

Do not make this block the core Memory Galaxy release.

---

## Phase 6 — Semantic Retrieval

Only after the basic system works.

Potential:

```text
keyword search
      +
semantic/vector search
      ↓
hybrid results
```

Keep this entirely behind the existing KnowledgeProvider contract.

---

# 20. End-to-End Acceptance Test

The implementation is successful when this works:

```text
1. Start TimeKeeper.

2. Start Obsidian with Memory Galaxy enabled.

3. TimeKeeper reports Galaxy connected.

4. Create a Trail of Thought entry.

5. Click "Promote to Galaxy".

6. The item appears inside Obsidian's Galaxy Inbox.

7. Edit and verify it in Obsidian.

8. Search for the concept through TimeKeeper.

9. TimeKeeper retrieves the Galaxy item.

10. Create another TimeKeeper Project.

11. Search for the same concept.

12. The new Project can retrieve the same Galaxy knowledge.

13. The original Galaxy item did not need to be edited or linked
    manually to the new Project.
```

The implementation is not accepted until these boundary cases also pass:

- TimeKeeper starts and remains fully usable with Obsidian absent.
- Obsidian starts after TimeKeeper and reconnects without restarting TimeKeeper.
- Obsidian restarts between Create and acknowledgement.
- Repeating promotion of one Trail item returns one Galaxy ID and creates no duplicate.
- A timeout after file creation is reconciled with the same promotion key.
- Two Trail items with similar titles retain distinct immutable Galaxy IDs.
- Manually edited Markdown is returned through Get/Search with updated content hash.
- Malformed frontmatter is rejected or quarantined without taking down the API.
- A caller without the capability token is rejected.
- An incompatible protocol version produces a typed compatibility error.
- A sensitive Trail item is visibly flagged or blocked before cross-project promotion.
- Search returns bounded summaries, while Get returns full content only by ID.
- A corrupt/unavailable search index returns an error rather than an empty success.
- A different project can retrieve `visibility: galaxy` knowledge without a manual
  project link, while `visibility: private` knowledge remains excluded.
- Every failed provider request leaves an observable, truthful promotion state.

---

# 21. Verification Matrix

| Boundary | Required evidence |
|---|---|
| TimeKeeper provider | unit tests with mock status, timeout, typed errors, and idempotent Create |
| `knowledge_links` | migration test, uniqueness test, retry/reconciliation test |
| Obsidian API | authenticated route tests, body/limit tests, protocol negotiation tests |
| Markdown repository | schema, atomic write, duplicate-ID quarantine, malformed-file tests |
| Promotion UI | redaction/visibility preview and truthful pending/created/failed states |
| Search | deterministic ordering, bounded response, full-content Get path |
| Restart behavior | Obsidian restart and lost-acknowledgement integration fixture |
| Privacy | sensitive-marker rejection/redaction test and visibility filtering |

Source-level tests are not sufficient. The integration must also be exercised
against a real local Obsidian plugin process or an equivalent authenticated
loopback fixture.

---

# Final Boundary

Keep these responsibilities clear:

```text
TIMEKEEPER
Projects
Tasks
Sprints
Execution history
Trail of Thought
Knowledge queries
Promotion

OBSIDIAN MEMORY GALAXY
Long-horizon memory
Cross-project knowledge
RAG material
Skills
Harnesses
Research
Knowledge relationships
Retrieval/indexing
```

The key rule is:

> TimeKeeper manages work. Memory Galaxy manages reusable knowledge.

Do not expand this implementation into a generic plugin platform unless a future requirement actually proves that necessary.