# Plugin architecture

TimeKeeper Core is stable. Optional functionality is a plugin, and
installing one must never require editing core packages.

This document is the contract for writing one.

## The rule

```text
Core defines the seam.  Plugins implement it.  Core never imports a plugin.
```

The process-level boundary between the two boxes is
[two-box-architecture.md](two-box-architecture.md).

A package under `internal/api`, `internal/model`, `internal/store` or
`internal/cli` must never import a plugin package. If you find yourself
adding a plugin import to one of those, the seam is being bypassed.

## Two integration paths

### Supported Store path: external plugin box

Store-installed plugins implement an existing stable capability in a separate
process and connect through `timekeeper-bridge/v1`. This path requires no Core
source change, no Core import, and no Core rebuild per plugin. The Store package
is staged and verified; a trusted Core launcher supplies the executable,
loopback connector, timeout, filesystem root, and granted capabilities.

Read [store-developer-guide.md](store-developer-guide.md) for the complete
package and bridge contract.

### Legacy in-process path: built-in development wiring

The registry below documents the older in-process development seam. It is useful
for Core-owned adapters and tests, but it is **not** the Store installation path:
its registration package is linked into the server binary and therefore requires
a Core build change. Do not publish a Store package that relies on this path.

## The in-process seam

```text
internal/plugin            registry — owned by core
internal/plugin/<name>     registration adapter — Core-owned wiring
internal/<capability>      the interface a plugin implements
```

Three layers, one direction of dependency:

```text
cmd/server  ──> internal/plugin            (asks for a kind)
                     │
                     └─ registration pkg ──> the plugin itself
```

`cmd/server` imports a plugin's *registration* package only. That package is
acceptable for Core-shipped functionality, not for an independently installed
Store plugin.

## Kinds

A `Kind` is a capability core promises to understand.

| Kind        | Interface                          | Package                  |
| ----------- | ---------------------------------- | ------------------------ |
| `knowledge` | `knowledge.KnowledgeProvider`      | `internal/knowledge`     |

Adding a kind is a core change. Adding a plugin for an existing kind
is not.

## Writing a plugin

Two files. Neither touches core.

**1. Implement the interface** in your own package:

```go
package mygalaxy

func (p *Provider) Status(ctx context.Context) (knowledge.ProviderStatus, error) { ... }
func (p *Provider) Search(ctx context.Context, q knowledge.KnowledgeQuery) ([]knowledge.KnowledgeResult, error) { ... }
func (p *Provider) Get(ctx context.Context, id string) (knowledge.KnowledgeItem, error) { ... }
func (p *Provider) Create(ctx context.Context, c knowledge.KnowledgeCandidate, key string) (knowledge.KnowledgeItem, error) { ... }
```

**2. Register it** from a registration package:

```go
package mygalaxyplugin

func init() {
    plugin.Register(plugin.Descriptor{
        ID:   "vendor.my-galaxy",
        Kind: plugin.KindKnowledge,
        Name: "My Galaxy",
    }, Build)
}

func Build(config map[string]string) (any, error) {
    return mygalaxy.New(config["endpoint"]), nil
}
```

**3. Import it once** in `cmd/server/main.go`:

```go
_ "github.com/JustSebNL/timekeeper/internal/plugin/mygalaxyplugin"
```

That import is the only core edit an install requires — one line, and
only because Go links plugins at compile time.

## Error contract

Plugins report failures as `*knowledge.ProviderError`:

```go
&knowledge.ProviderError{Code: "provider_unavailable", Retryable: true, Status: 503}
```

Core classifies through `knowledge.AsProviderError`,
`knowledge.IsRetryable`, `knowledge.ErrorCode` and
`knowledge.HTTPStatus`. It never type-asserts a plugin's own error
type, so a plugin may keep its internal error shape as long as it
wraps or aliases this one.

`ProviderError` exposes only a code, a message, a retry hint and a
status — never an endpoint, a token, a path or a raw response body.

## Degradation

A plugin is optional. Every failure mode degrades, none is fatal:

| Situation            | Result                                  |
| -------------------- | --------------------------------------- |
| plugin not installed | typed "not installed" error             |
| build fails          | logged, capability unavailable          |
| wrong type returned  | logged, capability unavailable          |
| runtime call fails   | `ProviderError` mapped to a status code |

Core starts and runs with zero plugins.

## Store delivery

The Store ships signed `.tkp` packages. A package declares its plugin
id, kind and permissions in its manifest; core grants capabilities and
never lets a package widen them itself. See
[extensions.md](extensions.md) and [package-format.md](package-format.md).

## Testing the seam

`internal/plugin/registry_test.go` proves a provider core has never
heard of can be registered, built and used through the interface.
`cmd/server/plugin_wiring_test.go` proves the real binary registers
the shipped plugin, so dropping the import fails a test rather than
silently disabling the feature.
