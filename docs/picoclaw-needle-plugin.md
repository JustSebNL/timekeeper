# PicoClaw Needle plugin and TimeKeeper plugin store

> Status: Implemented first plugin/store slice; production deployment is not
> verified.

## What was added

The extracted edge capability is now an optional PicoClaw process plugin:

```text
dev-ops/picoclaw/plugins/needle/plugin.py
dev-ops/picoclaw/plugins/needle/test_plugin.py
dev-ops/picoclaw/plugins/needle/plugin.json
dev-ops/picoclaw/plugins/needle/cactus-needle-0.1.0.tar.gz
```

It uses PicoClaw's existing JSON-RPC 2.0 stdio hook protocol. It does not add a
second plugin ABI.

The plugin:

- calls local Needle only when `cactus-needle` is installed;
- classifies the latest user intent with one bounded tool schema;
- injects a small, explicitly labelled, untrusted classification into the next
  LLM request;
- redacts likely credentials and truncates input/output;
- fails open when Needle is absent or fails;
- never executes shell commands;
- never performs network calls;
- never writes arbitrary files;
- never authorizes tools;
- never mutates TimeKeeper or ShadowBox state.

Needle is therefore an optional intelligence layer, not an execution authority.

## Run locally

Install the optional Needle runtime in the environment used by the plugin:

```bash
python3 -m pip install cactus-needle
```

The plugin disables Needle telemetry and model downloads by default. Provision
an offline engine on the host first (for example with the Needle `fetch`
workflow), or explicitly opt into model downloads through a separately
reviewed environment policy. The plugin itself should not silently reach the
network during an agent turn.

Configure PicoClaw's existing process-hook system. The exact path should be
resolved from the installed plugin directory; do not hardcode this repository
path in a public installation:

```json
{
  "hooks": {
    "enabled": true,
    "processes": {
      "cactus-needle": {
        "enabled": true,
        "priority": 100,
        "transport": "stdio",
        "command": ["python3", "/path/to/plugin.py"],
        "dir": "/path/to/needle-plugin",
        "intercept": ["before_llm"]
      }
    }
  }
}
```

Optional environment controls:

```text
NEEDLE_PLUGIN_MAX_INPUT_CHARS=12000
NEEDLE_PLUGIN_MAX_OUTPUT_CHARS=2400
NEEDLE_PLUGIN_MIN_CONFIDENCE=0.70
```

For deterministic protocol tests only:

```text
NEEDLE_PLUGIN_FAKE=1
```

Do not enable the fake mode in production.

## Plugin store contract

TimeKeeper exposes an optional read-only registry when started with:

```bash
timekeeper --plugin-store-root /srv/timekeeper/plugins
```

or:

```text
TIMEKEEPER_PLUGIN_STORE_ROOT=/srv/timekeeper/plugins
```

The registry layout is:

```text
/srv/timekeeper/plugins/
└── cactus.needle/
    ├── plugin.json
    └── cactus-needle-0.1.0.tar.gz
```

Routes:

```text
GET /api/v1/plugins
GET /api/v1/plugins/{pluginID}
GET /api/v1/plugins/{pluginID}/artifact
```

The edge client defaults to:

```text
https://time.mngr.eu.org/api/v1/plugins
```

List available metadata with:

```bash
edge --list-plugins
```

Explicitly download one selected artifact without unpacking or executing it:

```bash
edge --download-plugin cactus.needle
```

The destination can be changed with `--plugin-dir`. The artifact is checksum
verified and written with private permissions; a later installation step must
review its manifest permissions and configure the PicoClaw process hook.

Use another store during local testing with:

```bash
edge --list-plugins --plugin-store http://127.0.0.1:1618
```

or:

```text
AI_HUB_PLUGIN_STORE_URL=https://time.mngr.eu.org
```

The current public URL was checked during implementation and returns a
placeholder page rather than the plugin API. The client and TimeKeeper routes
are ready, but the remote deployment must be configured before this command can
return a real index.

## Manifest requirements

Every manifest must use:

```text
schema_version: timekeeper-plugin-manifest/v1
id: lowercase stable ID
version: immutable plugin version
runtime: runtime name
entrypoint: relative entrypoint
protocol: supported process protocol
artifact: relative artifact filename
sha256: lowercase SHA-256 of artifact
```

Artifacts are served only from the plugin's own directory. Absolute paths,
`..` traversal, invalid IDs, cross-origin artifact URLs, missing checksums, and
checksum mismatches are rejected.

The edge client currently lists and validates metadata. It does not silently
install or execute remote artifacts. Installation remains an explicit,
reviewable operation, which is the correct default for code capable of running
inside an agent environment.

## TimeKeeper / ShadowBox boundary

```text
TimeKeeper plugin store:
  manifests, artifact delivery, checksums, versions

Edge runtime:
  local journal, plugin discovery, event provenance

Needle plugin:
  bounded classification only

ShadowBox:
  installation policy, sandboxing, runtime restrictions, execution

TimeKeeper:
  project/task/sprint authority and durable provenance
```

A future installer should download an artifact, verify its checksum, unpack it
into a derived local plugin directory, inspect declared permissions, and only
then add a process-hook configuration. It must not turn a registry listing into
automatic code execution.

## Verification performed

Python:

```text
python3 -m py_compile plugins/needle/plugin.py plugins/needle/test_plugin.py
3 protocol tests passed
```

PicoClaw:

```text
go test ./internal/pluginstore ./internal/journal ./cmd/edge
PASS
go vet ./internal/pluginstore ./internal/journal ./cmd/edge
PASS
standalone edge build and journal smoke test
PASS
```

TimeKeeper:

```text
go test ./internal/api -run 'TestPluginStore' -count=1
PASS
```

Full PicoClaw verification remains subject to the existing Matrix/mautrix
`libolm` baseline build blocker documented in
[`picoclaw-strip-audit.md`](picoclaw-strip-audit.md).
