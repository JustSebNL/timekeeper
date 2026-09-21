# TimeKeeper extensions

TimeKeeper Core ships **extension support and no optional
extensions**. Optional functionality is a Store package; something
belongs in Core only when TimeKeeper no longer works normally without
it.

How a plugin is built and wired: [plugin-architecture.md](plugin-architecture.md).

## What is an extension

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
| `network.access`    | outbound network              |
| `filesystem.read`   | read files the user grants    |
| `filesystem.write`  | write files the user grants   |

Core enforces the grant. A package cannot widen its own permissions.

## Remote JavaScript

Arbitrary remote JavaScript never runs with unrestricted privileges.
Preferred shape:

```
remote service → data/API result → trusted TimeKeeper renderer
```

If a downloaded extension executes JavaScript, it runs inside the
extension runtime under the permission model above.

## Install from the CLI

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

## Local development against the Store

The private `TimeKeeper-Store` repository can seed real signed
packages so the full path is testable offline. See
`docs/auth-protocol.md`.
