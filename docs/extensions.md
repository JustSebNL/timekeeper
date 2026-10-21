# TimeKeeper extensions

Store documentation hub: [store.md](store.md).

TimeKeeper Core ships **extension support and no optional
extensions**. Optional functionality is a Store package; something
belongs in Core only when TimeKeeper no longer works normally without
it.

How a Store plugin is developed and connected without changing Core:
[store-developer-guide.md](store-developer-guide.md). How the two boxes talk:
[two-box-architecture.md](two-box-architecture.md).

## Examples of possible extensions

The entries below are illustrative plugin areas, not an available catalogue or a committed roadmap. No optional Store package is currently offered by this repository's checked-in client; some examples have separate implementation projects or partial provider code. When a specific optional feature is approved, track it as a Store plugin with a named owner and acceptance criteria rather than silently expanding Core. The current approval/deferral decisions are recorded in [plugin-candidate-triage.md](plugin-candidate-triage.md).

```
Memory Galaxy          cross-project knowledge graph
Obsidian               promote checkpoints into a vault
Shadowbox              sandboxed verification sprints
Cloud backup           Google Drive, OneDrive, Dropbox, Mega
AI tooling             provider-specific agents
advanced metrics       specialised analytics
```

## Permissions

An extension requests capabilities. It never receives unrestricted
application access.

```json
{
  "id": "memory-galaxy",
  "permissions": ["projects.read", "memory.read", "memory.write"]
}
```

| Permission          | Grants                        |
| ------------------- | ----------------------------- |
| `projects.read`     | read projects and categories  |
| `projects.write`    | mutate projects               |
| `tasks.read`        | read tasks and subtasks       |
| `tasks.write`       | mutate tasks                  |
| `history.read`      | read audit history            |
| `totb.read`         | read the Trail of Thought     |
| `totb.write`        | post to the Trail of Thought  |
| `memory.read`       | read memory surfaces          |
| `memory.write`      | write memory surfaces         |
| `network.egress`   | Core-controlled outbound access to an explicit allowlist; default denied |
| `filesystem.read`   | read files the user grants    |
| `filesystem.write`  | write files the user grants   |

Core enforces the grant. A package cannot widen its own permissions.

## Outbound connectivity is default-deny

Plugins do not receive raw socket access, unrestricted DNS, or inherited proxy
credentials. The absence of `network.egress` means no outbound network.

When a product genuinely needs network access, its release must declare a
narrow egress policy that Core approves separately from the package manifest:

```yaml
egress:
  mode: allowlist
  destinations:
    - host: api.example.com
      ports: [443]
      methods: [GET, POST]
  max_requests_per_minute: 60
  max_upload_bytes_per_minute: 1048576
  max_download_bytes_per_minute: 10485760
  max_response_bytes: 2097152
  max_concurrent: 2
```

This is a design contract, not a permission string a plugin can grant itself.
The Core launcher establishes a Core-owned egress broker/connector. The plugin
can request only connector operations; Core resolves the destination, applies
the policy, enforces quotas, and performs the outbound request. OS-level
firewall/network isolation is defense in depth, never a substitute for Core
mediation. If Core cannot establish the broker, the plugin does not activate.

The connector must:

- resolve DNS itself and reject loopback, link-local, private, multicast,
  metadata-service, and reserved destinations;
- prevent DNS rebinding and IP-literal bypasses;
- permit only exact hosts, ports, methods, and redirect targets on the policy;
- cap upload bytes, download bytes, response size, rate, and concurrency;
- revoke the connector on quota exhaustion or suspicious behavior;
- record destination, method, byte counts, status, quota, and denial reason;
- redact request/response bodies by default and never log credentials;
- keep Store/Core tokens out of the plugin process.

A plugin must not bypass the connector by opening its own socket, invoking a
system networking tool, changing proxy environment variables, or requesting a
new destination at runtime. Such behavior is a containment failure and causes
shutdown plus an audit event.

## Remote JavaScript

Arbitrary remote JavaScript never runs with unrestricted privileges.
Preferred shape:

```
remote service → data/API result → trusted TimeKeeper renderer
```

If a downloaded extension executes JavaScript, it runs inside the
extension runtime under the permission model above.

## Install from the CLI

This is an optional developer/operator flow, not part of first use. It requires a
separately deployed Store and `TIMEKEEPER_STORE_URL`; the checked-in default is
a reserved `.example` placeholder and does not reach a live catalogue.

```bash
tk store login you@example.com --password '<password>'
tk store catalogue
tk store install extension.memory-galaxy
```

Installed packages land under the TimeKeeper state directory:

```
.timekeeper/extensions/<product-id>/
```

That directory is gitignored: installed extensions are runtime
artifacts, not repository content, and the public repository must
never carry Store payloads.

## Install is not activation

A successful Store install means only: entitlement accepted, package
downloaded, checksum and detached signature verified, and the bounded
archive extracted. **It does not execute the package or import it into
Core.**

Core and an installed extension are separate boxes. Core owns Projects,
SQLite, ToTb, permissions, and lifecycle authority. The extension runs in
its own supervised process with its own memory. They may communicate only
through predetermined connectors:

- control tunnel: newline-delimited JSON over child-process stdio for
  spawn, handshake, health, and shutdown;
- capability tunnel: JSON over an explicitly assigned numeric-loopback
  HTTP port for declared work capabilities.

The bridge protocol is `timekeeper-bridge/v1`. Capabilities are frozen at
handshake and every call is checked against that allowlist. An extension
never receives a Core database handle, Core pointer, arbitrary filesystem
path, request-selected command, or unrestricted network bridge. A valid
signature proves provenance; it does not grant execution permission.

The current CLI install path is therefore staging-only until the supervised
bridge launcher and connector-specific capability adapters are wired and
verified. If the bridge is unavailable, Core reports the extension as
unavailable and continues operating normally. See
[`two-box-architecture.md`](two-box-architecture.md).

## Local development against the Store

The private `TimeKeeper-Store` repository can seed real signed
packages so the full path is testable offline. See
`docs/auth-protocol.md`.
