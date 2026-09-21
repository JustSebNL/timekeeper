# Two-box architecture

TimeKeeper is two boxes that may influence each other **only through
predetermined tunnels**. Neither box reaches into the other's memory,
types, database or filesystem.

```text
+---------------------------+          +---------------------------+
|  CORE BOX                 |          |  PLUGIN BOX               |
|                           |          |                           |
|  projects, tasks, sprints |  control |  one capability           |
|  SQLite, ToTb, history    | <------> |  (knowledge, backup, ...) |
|  CLI, dashboard, guardian |   stdio  |                           |
|                           |          |  own process, own memory  |
|  owns all authority       |<-------->|  gets only what it declares|
+---------------------------+ capabili- +---------------------------+
                               ty HTTP
                              loopback
```

The plugin box is a separate process. A plugin that crashes, hangs or
misbehaves cannot corrupt core — the worst case is a dead tunnel, and
core degrades to "unavailable".

## Why processes, not dynamic loading

Go's `plugin` package needs cgo; this builds with `CGO_ENABLED=0` on
windows/amd64. In-process loading is therefore unavailable.

That constraint is a feature. A process boundary gives real isolation
and lets the existing `internal/supervisor` (job objects on Windows,
process groups on Unix) contain CPU, memory and lifetime.

## The two tunnels

| Tunnel         | Transport                  | Carries                          |
| -------------- | -------------------------- | -------------------------------- |
| **control**    | newline JSON over stdio    | spawn, handshake, health, shutdown |
| **capability** | JSON over loopback HTTP    | the actual work                  |

Control is stdio because it is always available, needs no port and
cannot be reached from outside the machine. Capability calls are HTTP
because they reuse the loopback API surface `guardian` already speaks.

## Handshake

Core spawns the plugin box and sends `hello`. The plugin box answers
`hello_ok` with its identity and the capabilities it declares.

```jsonc
// core -> plugin
{"method":"hello","params":{"protocol_version":"timekeeper-bridge/v1",
 "plugin_id":"extension.memory-galaxy","kind":"knowledge"}}

// plugin -> core
{"method":"hello_ok","params":{"protocol_version":"timekeeper-bridge/v1",
 "plugin_id":"extension.memory-galaxy","declared_capabilities":["knowledge.search"]}}
```

Refused, before any work happens:

- protocol version mismatch
- plugin answering for a different id than core asked for
- an empty or duplicate capability
- a capability origin that is not plain `http` on an explicit loopback port

`PerformHandshake` (core side) and `ServeHandshake` (plugin side) in
`internal/bridge` implement this.

## The rule

> Capabilities are declared once and frozen. A plugin cannot widen them
> after the handshake.

Core checks every call against the frozen allowlist. Asking for an
undeclared capability yields `ErrCapabilityUnknown`; the call never
reaches the plugin box.

## What a plugin never receives

A plugin box is never handed a `*store.Store`, a database handle, a core
struct, or a filesystem path it did not declare in its manifest. It gets
a config map and a loopback origin. Nothing else crosses.

## Degradation

Every failure is non-fatal to core:

| Situation            | Result                              |
| -------------------- | ----------------------------------- |
| plugin not installed | typed error, capability unavailable |
| handshake fails      | logged, capability unavailable      |
| tunnel dies          | `ErrClosed`, capability unavailable |
| call hangs           | timeout, capability unavailable     |

Core starts and runs with an empty plugin box.

## Status

The bridge is the contract and is implemented. The knowledge/Obsidian
capability is the first slice to move across it; see
[plugin-architecture.md](plugin-architecture.md) for the registration
side that predates this split.

## Testing

`internal/bridge/bridge_test.go` covers the agreed handshake and every
refusal path. Note for test authors: `net.Pipe` is unbuffered and
synchronous — a send blocks until a reader is waiting, so start the
reader goroutine **before** sending or the test hangs.
