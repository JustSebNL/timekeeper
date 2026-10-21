# Optional Store plugin candidate triage

Status: current backlog decision for T-10476
Last reviewed: 2026-10-02

This document turns the illustrative extension areas in [extensions.md](extensions.md)
into explicit decisions. It is not a Store catalogue and does not claim that the
approved candidates are installed or live.

## Decision rules

A candidate may become a Store plugin only when all of these are true:

1. TimeKeeper remains fully usable without it.
2. The package has a narrow owner and a concrete user outcome.
3. Core can enforce its capabilities; the plugin cannot widen them.
4. Credentials, filesystem access, network egress, and model calls have a named
   boundary before activation.
5. The result has a dedicated tracker item instead of an untracked roadmap promise.

## Candidates

| Candidate | Decision | Existing tracker | Rationale and boundary |
| --- | --- | --- | --- |
| Obsidian Memory Galaxy | **Approved; continue existing work** | P-10305; live smoke remains T-10321; Markdown semantics remain T-10469 | Preserves useful cross-project knowledge in a user-controlled vault. The provider contract, promotion gate, and repository semantics are partially implemented in source; live Obsidian verification is still outstanding. |
| ShadowBox workspace | **Approved; continue existing work** | P-10289 | Sandboxed verification and isolated model/tool execution are useful for optional workflows. It remains a separate runtime boundary and must not become a second TimeKeeper database. |
| Jev Category/Sprint routing | **Approved; continue existing work** | T-10468 | A bounded typed decision provider can reduce manual hierarchy selection for agents. Core now has the proposal policy and `routing.Provider` seam, but there is no live TypeSafe client or Store plugin yet. |
| Cloud backup | **Deferred** | No new task created | Backup can be valuable, but provider credentials, encrypted snapshots, restore testing, and managed egress are prerequisites. A future proposal should cover only encrypted vault/config snapshots for a named provider; it must not become an always-on sync service by default. |
| Broad AI tooling | **Non-goal as a generic bucket** | No new task created | "AI tooling" is too broad to enforce safely. Narrow integrations may be proposed separately, as Jev routing was, with a named provider, capability list, credential boundary, and failure mode. |
| Advanced metrics | **Deferred** | No new task created | Extra analytics are useful only when backed by real, documented events. Create a proposal only after naming the source data, retention rules, dashboard/API surface, and a metric that cannot be represented by existing usage summaries. |

## Result

No additional tracker tasks were created. The three approved candidates already
have authoritative work items; duplicating them would fragment ownership. Deferred
and non-goal candidates may return only through a new, narrowly scoped proposal
that satisfies the decision rules above.

This triage does not authorize remote access, always-on synchronization,
credential collection, or plugin execution. Store installation, verification,
activation, and runtime calls remain separate gates.
