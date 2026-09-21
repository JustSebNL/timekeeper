# PicoClaw reduction audit for TimeKeeper / ShadowBox

> Status: Source-verified baseline and design audit.
>
> Primary plan: [`../dev-ops/picoclaw-strip-plan-timekeeper-shadowbox.md`](../dev-ops/picoclaw-strip-plan-timekeeper-shadowbox.md)
>
> This document records what was verified before reducing the cloned PicoClaw
> tree. It does not claim that the reduction is implemented.

## Repositories and toolchain

PicoClaw clone:

```text
/mnt/d/dev/codebase/dev/TimeKeeper/dev-ops/picoclaw
```

PicoClaw module:

```text
github.com/sipeed/picoclaw
go 1.25.13
```

Observed upstream checkout identity:

```text
commit bbf6893c
feat(models): add configurable default fallback chain (#3200)
origin https://github.com/sipeed/picoclaw
```

Needle reference repository:

```text
/mnt/h/dev/repos/needle
```

Needle package identity:

```text
cactus-needle 2.0.12
Python >= 3.9
Apache-2.0
```

Available Go toolchain:

```text
go version go1.26.1 windows/amd64
/mnt/c/Program Files/Go/bin/go.exe
```

WSL does not currently have a `go` executable on `PATH`.

## Working-tree safety

The PicoClaw clone has a heavily modified working tree. Git reports hundreds
of modified files, including core packages, providers, channels, web assets,
and tests. The tree must not be reset, cleaned, or mass-deleted as part of the
reduction. New work should be additive or isolated until the source ownership
of the existing modifications is understood.

## Baseline verification

Focused existing-package tests pass through the available Windows Go toolchain:

```text
go test ./pkg/events ./pkg/identity ./pkg/bus
ok  github.com/sipeed/picoclaw/pkg/events
ok  github.com/sipeed/picoclaw/pkg/identity
ok  github.com/sipeed/picoclaw/pkg/bus
```

The full baseline suite did not produce a result within the command timeout.
The first direct build also failed before producing a binary because the
current Windows build selected the Matrix/mautrix `libolm` path, while the
module reports that all files in `crypto/libolm` are excluded by build
constraints:

```text
maunium.net/go/mautrix/crypto/libolm:
build constraints exclude all Go files
```

This is an environment/build-surface constraint, not evidence that the entire
project is broken. A later baseline must either provide the required libolm
build surface or build a reduced target that does not import Matrix support.
No source was changed to hide this failure.

## Existing capabilities that match KEEP

### Event bus

`pkg/events/` already contains an in-process event bus with subscriptions,
filters, backpressure behavior, non-blocking publish, close semantics, and
statistics. This is a strong reusable base for the edge observer layer.

### Runtime event logging

`pkg/agent/runtime_event_logger.go` already subscribes to runtime events,
filters by severity and event patterns, supports bounded buffering, and logs
asynchronously through the existing logger. It is not yet the immutable SQLite
journal described by the reduction plan.

### Identity and agent lifecycle

The current agent packages already contain session/turn and sub-agent concepts.
These should be mapped to the planned actor/trace envelope rather than
reimplemented blindly.

### Security and configuration surfaces

The repository already contains sensitive-data filtering, configuration,
provider abstractions, tool registries, and isolation-related packages. These
must be reviewed for reuse, not assumed to match the reduced runtime boundary.

## Existing capabilities that should be reduced or excluded from the edge

The current tree is much larger than the proposed infrastructure runtime:

```text
cmd/                  102 Go files
pkg/                  687 Go files
pkg/agent              93 Go files
pkg/channels          123 Go files across 21 channel packages
pkg/providers          84 Go files across 12 provider packages
pkg/tools              87 Go files
pkg/skills             15 Go files
web/                   74 Go files
```

The plan's first reduction targets are therefore credible:

- consumer channel integrations;
- web UI and launcher;
- media/audio/vision processing;
- provider zoo and model routing;
- cron and assistant heartbeat task execution;
- browser/search tools;
- direct shell execution from the edge runtime;
- generic file writing;
- personal-assistant memory and skills.

These should be removed or isolated only after a dependency map and a buildable
reduced target exist. Deleting them first would turn the current dirty clone
into a debugging landfill. Nobody needs that hobby.

## Needle findings

Needle is not a Go package. The reference repository is a Python package with
an optional native engine fetched/cached for inference. Its documented public
surface includes:

- `needle.Needle(...)`;
- `agent.complete(...)` for one constrained turn;
- `agent.run(...)` for a tool-calling loop;
- `needle.extract(...)` for typed extraction;
- JSON-schema-like tool declarations;
- `confidence` on base-model tool calls;
- empty-call refusal when no declared tool applies.

The package declares `huggingface_hub` as its runtime dependency. The README
states that the model engine is a single binary and that inference is offline
after the engine is cached, but the Python package remains the reference
integration surface.

Consequences for the reduction:

1. Do not import Needle into the Go edge binary.
2. Keep the edge runtime functional with Needle disabled or absent.
3. Treat Needle as an optional local sidecar/bridge, or use a future standalone
   native/WASM boundary if the project later needs a no-Python deployment.
4. Give Needle only narrow correlation/extraction tools; never shell, browser,
   arbitrary file-write, or generic network tools.
5. Persist raw events before Needle enrichment and keep model-generated links
   separate from immutable content.
6. Do not expose secrets or full transcripts to Needle.

## Recommended first implementation boundary

Do not start by deleting PicoClaw packages. Start with an additive, buildable
edge slice:

```text
cmd/edge/
internal/journal/
internal/envelope/
internal/actor/
internal/relay/       (initially a no-op/local sink)
internal/needle/      (interface + disabled/mock implementation only)
internal/timekeeper/  (read/link boundary, no lifecycle mutation)
internal/shadowbox/   (submission/event-link interface, no shell path)
```

The first slice should:

1. receive a typed runtime event;
2. assign a stable local sequence number;
3. commit the raw event to SQLite WAL;
4. acknowledge the event independently of relay or Needle;
5. expose a replay query for unacknowledged events;
6. preserve agent/session/turn/parent/trace IDs;
7. run with Needle disabled;
8. avoid importing channel, provider, web, or Matrix packages.

This isolates the new binary from the current Matrix/libolm build failure and
creates the journal contract before any large removal.

## First additive slice implemented

The first journal slice now exists without deleting or resetting PicoClaw
source:

```text
internal/journal/journal.go
internal/journal/journal_test.go
cmd/edge/main.go
```

Implemented behavior:

- SQLite WAL setup with bounded busy timeout;
- immutable raw event JSON plus queryable identity columns;
- local autoincrement sequence;
- idempotent append by event ID;
- bounded pending replay;
- idempotent acknowledgements;
- reopen persistence coverage;
- standalone edge bootstrap that does not import channel/provider/web/Matrix
  packages.

Focused tests and the edge build passed through the Windows Go toolchain. A
binary smoke test opened a repository-local journal and was then removed. The
full PicoClaw build remains blocked by the existing Matrix/libolm build
constraint described above.

## Decision boundaries

```text
Hermes:
  thinks, plans, uses general tools, and spawns agents.

Edge runtime:
  observes, journals, assigns identity, correlates, and relays.

Needle:
  optional narrow structured correlation/extraction only.

ShadowBox:
  authorizes and executes commands, queues, retries, sandboxes, verifies.

TimeKeeper:
  owns projects, categories, tasks, sprints, progress, time, snapshots,
  lifecycle, and durable work provenance.

AI-Hub Base / MariaDB / LanceDB / Obsidian:
  central observability, raw history, retrieval, and curated knowledge.
```

The edge runtime must not become a replacement TimeKeeper, a replacement
ShadowBox, or an autonomous memory curator.

## Next verified work sequence

1. Preserve this baseline and isolate the dirty PicoClaw state.
2. Inspect the existing runtime event envelope and identity fields.
3. Define the minimal SQLite journal schema and migration behavior.
4. Add journal tests using temporary databases and crash/replay scenarios.
5. Add a standalone `cmd/edge` build that excludes channel/provider/web trees.
6. Add disabled/mock Needle and TimeKeeper/ShadowBox interfaces.
7. Verify the edge binary on the available Go toolchain.
8. Only then begin removing obsolete PicoClaw surfaces in small, independently
   tested slices.

## Explicit blockers

- Full PicoClaw baseline build is blocked on the current Matrix/mautrix
  `libolm` build constraints under the available Windows toolchain.
- The full test command timed out without a usable result.
- The PicoClaw working tree is already heavily modified, so ownership and
  source provenance must be established before destructive cleanup.
- Needle's current public integration is Python/native-engine based; a pure Go
  embedding is not established by the reference repository.
