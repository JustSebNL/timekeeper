# Automatic FASE-ID Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Assign and cache a Core-owned FASE-ID automatically on first CLI use, without an enrollment command.

**Architecture:** Add a durable fingerprint registry and bounded Core-owned cache. Both executable client entry points use a shared identity bootstrap, then send the resolved FASE-ID through the existing activity/Guardian path. Runtime-session fingerprints and process-host observations have explicitly different scopes.

**Tech Stack:** Go 1.26.1, existing modernc SQLite Store, net/http, existing golang.org/x/sys for Windows process metadata.

**Spec:** [FASE identity design](../../fase-identity-design.md)

## Approved conversation amendments (2026-09-26)

These supersede the earlier numeric ID examples below. Use
`FASE-ID.<permanent mixed-alphanumeric fingerprint key>.<UTC month>.<UTC year>`.
Add immutable reserved-key storage, month-of-use aliases, masked Agent Profiles,
reported provider/model/runtime metadata and immutable event attribution.
Add exclusive parent-Task/Sprint execution claims before enabling universal auto-start.
Execution claims govern start/resume/complete, not permission to add notes,
evidence, child work, or other collaborative contributions. Ordered conflicting
edits and revision checks remain separate from execution claims.
No registration command, anonymous claim bypass, inactivity-driven release,
automatic identity merging, commits, or live deployment.

Task 1 now creates `fase_agents`, `fase_reserved_keys`, and `fase_aliases` rather
than the superseded numeric registry schema. `AgentIdentity` returns permanent
`agent_key` and monthly `fase_id`; internal work-state IDs use the permanent key.
Test same fingerprint over two months, skipped months, concurrent allocation,
forced key collision, invalid metadata and restart persistence.

Task 2 still bootstraps both client entry points, but attaches the stable key and
a domain-separated activity session token, not the registry fingerprint.
Core must compute the current alias for event attribution. Profiles
never expose the full fingerprint. CLI-generated model metadata is reported.

Task 3 includes two additional deliverables before final verification:

- Transactional ownership in `internal/store/fase_claims.go`, enforced inside
  existing Sprint/Task Store transactions, plus API claim/release routes and
  CLI commands. Tests must reproduce competing claims, sibling-Sprint bypass,
  anonymous mutation, owner retry, release-while-active, and monthly rollover.
- `internal/api/fase_profiles.go` profile/alias lookup and `web/fase-profile.js`
  profile rendering. Extend project event JSON with immutable actor attribution
  and link it from project/recent activity rendering using safe DOM APIs. Test
  masked fingerprint, unknown/reported metadata, monthly alias history, and
  historical event snapshots after metadata changes.

Implementation is explicitly authorized by the user's request to document and
implement the discussion. Amendments are recorded here rather than requiring
another design approval loop.

## Global Constraints

- There is no registration CLI command.
- Identity discovery and cache hits never count as material progress.
- Do not derive identity from a shared project file.
- Cache publication follows commit.
- IDs are opaque, immutable, and never reused.
- Preserve legacy rows and history.
- Source tests are not live proof.
- Preserve the dirty worktree; no automatic commits, deployment, extra live server, or database replacement.

## Review Focus

1. Simultaneous first commands must converge on one durable assignment.
2. Cache eviction/restart must not change an assigned FASE-ID.
3. Two agents in one host/workspace must not be claimed distinct without a real session discriminator.
4. An offline/old Core must not allow anonymous project mutations or prevent recovery diagnostics.
5. A wake adapter must inherit its watched identity, not its poller's identity.

## Task 1: Durable registry and shared cache

Files: create `internal/model/agent_identity.go`, `internal/store/agent_identity.go`,
`internal/store/agent_identity_test.go`; modify `internal/store/store.go`.

Interfaces:

```go
type AgentIdentityInput struct {
    Fingerprint string `json:"fingerprint"`
    Runtime string `json:"runtime"`
    IdentityScope string `json:"identity_scope"`
}
type AgentIdentity struct {
    AgentID string `json:"agent_id"`
    Fingerprint string `json:"fingerprint"`
    Runtime string `json:"runtime"`
    IdentityScope string `json:"identity_scope"`
}
// Store method; context carries cancellation, not caller identity.
func (s *Store) ResolveAgentIdentity(ctx context.Context, in model.AgentIdentityInput) (model.AgentIdentity, error)
```

- [ ] Add Store tests with real temporary databases. The first test should fail
  because there is no registry yet:

```go
func TestFASEIdentityReusesFingerprint(t *testing.T) {
    s := openPulseStore(t)
    input := model.AgentIdentityInput{Fingerprint: strings.Repeat("a",64), Runtime:"custom", IdentityScope:"runtime-session"}
    first, err := s.ResolveAgentIdentity(t.Context(),input)
    if err != nil { t.Fatal(err) }
    second, err := s.ResolveAgentIdentity(t.Context(),input)
    if err != nil || first.AgentID != second.AgentID || !strings.HasPrefix(first.AgentID,"FASE-") { t.Fatalf("%+v %+v %v",first,second,err) }
}
```

- [ ] Run `go test ./internal/store -run TestFASE -count=1`; inspect expected red.
- [ ] Add schema above, Store-scoped cache and `ResolveAgentIdentity`. Hold the
  cache mutex across a miss resolution initially to coalesce first use. Validate
  lowercase SHA-256 fingerprint, runtime length 1..64 and the two allowed scopes.
  Use an insert-on-conflict transaction and select the committed existing/new row.

```sql
INSERT INTO agent_identities(fingerprint,runtime,identity_scope,created_at)
VALUES(?,?,?,?) ON CONFLICT(fingerprint) DO NOTHING;
SELECT identity_number,fingerprint,runtime,identity_scope
FROM agent_identities WHERE fingerprint=?;
```

- [ ] Implement LRU hit promotion/4096-entry eviction with `container/list` and a
  map inside the Store cache. Reject conflicting runtime/scope for an existing
  fingerprint. Do not cache failures; do not call activity recording here.
- [ ] Add tests for 32 concurrent resolutions, two different fingerprints,
  invalid fingerprints, metadata conflict, failed transaction followed by retry,
  cache hits, eviction and reopen. For cache-hit proof, acquire a cached mapping,
  close only the isolated test DB connection, and verify the same mapping resolves
  without a registry query; this does not claim the whole service works offline.
- [ ] Run focused tests then `go test ./internal/store -count=1`. Inspect the diff
  and post exact evidence to T-10422; do not commit unrelated user changes.

## Task 2: Automatic bootstrap in both CLI entry points

Files: create `internal/api/agent_identity.go`, `internal/api/agent_identity_test.go`,
`internal/cli/fase_bootstrap.go`, `internal/cli/fase_bootstrap_test.go`,
`internal/cli/fase_process_windows.go`, `internal/cli/fase_process_linux.go`,
`internal/cli/fase_process_other.go`; modify `internal/api/server.go`,
`internal/cli/agent_identity.go`, `internal/cli/http.go`, `cmd/tk/main.go`,
`cmd/server/client_dispatch.go` and their tests.

Interfaces:

```go
// Internal HTTP handshake; not a registration CLI command.
// POST /api/v1/agent-identities/resolve -> model.AgentIdentity
func RunTracked(args []string, out, errOut io.Writer, baseURL string) int
type fingerprintSource struct { Runtime, Session, Scope string }
func discoverFingerprintSource() (fingerprintSource, error)
func processSessionSource() (fingerprintSource, error)
```

- [ ] Write an API test posting the same valid fingerprint twice and another
  fingerprint once, using a real Store. Assert stable/distinct FASE-IDs and no
  work/progress lease created merely by registry lookup. Run it to expected red.
- [ ] Add bounded JSON handshake handler using the Store method. No auth authority
  is inferred from an ID/fingerprint. Use typed 400 validation/500 persistence errors.
- [ ] Write executable/bootstrap tests: first `list`, next `agent identity`, then
  another session's `list`. Assert two actual registry/work-state identities,
  not a mocked registration call. Include help as first invocation.
- [ ] Derive the fingerprint from a versioned, unambiguous encoding, not string
  concatenation without separators:

```go
encoded, err := json.Marshal([]string{"fase-v1", source.Runtime, source.Scope, source.Session})
if err != nil { return err }
sum := sha256.Sum256(encoded)
fingerprint := hex.EncodeToString(sum[:])
```

- [ ] Prefer the explicit runtime session contract, then verified Codex session
  variables. Keep provider discovery in this module so more host integrations
  do not modify Guardian. Never use a caller-supplied friendly label as proof of
  identity. Propagate runtime-session tokens automatically from the wake adapter.
- [ ] Implement process-session observation from OS metadata: Windows Toolhelp
  parent lineage plus GetProcessTimes creation time; Linux proc parent/starttime
  plus boot ID. Inspect only bounded ancestors and process metadata, never argv,
  environments of unrelated processes, or transcripts. Skip transient shell
  wrappers only while a valid non-system parent is observable; never select PID
  1 or the common OS desktop as an individual agent. Report process-session scope
  openly. Other platforms return an actionable host-integration diagnostic.
- [ ] Test PID reuse and shared-host cases using a pure fingerprint-source
  function with literal fixtures. Shared host without session tokens must remain
  explicitly process-scoped; two supplied session tokens must split identities.
- [ ] `RunTracked` resolves identity before calling the existing command dispatcher.
  Both binary entry points call it; do not merely modify HTTP mutations. Scope
  the resolved in-process identity to the selected Core URL so a second endpoint
  cannot inherit a previous assignment. Avoid environment mutation as storage.
- [ ] Make `agent identity/state/work/wait/idle/stop` consume the resolved identity.
  Attach it to ordinary HTTP requests through `diagnosticTransport`; registry
  handshake uses a raw bounded client to avoid recursion. Keep old `Run` available
  as the internal dispatcher; shipped executables cannot bypass `RunTracked`.
- [ ] Add failure tests with unavailable and old Core: requested project mutation
  never reaches its handler; help/doctor/service recovery display diagnostics and
  remain usable without pretending enrollment succeeded. No anonymous fallback.
- [ ] Run `go test ./internal/api ./internal/cli ./cmd/server ./cmd/tk -count=1`,
  inspect both entry-point diffs, and record evidence before closing the Sprint.

## Task 3: Guardian continuity, docs and complete verification

Files: modify `internal/cli/guardian_binding.go`, `internal/cli/guardian_wake.go`,
`internal/cli/guardian_portable_test.go`, `internal/cli/agent_activity_integration_test.go`,
`docs/current-guardian-input-flow.md`, `docs/agent-wake-adapter-protocol.md`,
`harness/SKILL.md`, `API.md`, `AGENT_INTEGRATION.md`.

Consumes: the registry, RunTracked, and existing RecordAgentActivityAt state machine.
Produces: verified first-use -> work -> idle -> nudge -> same-identity continuation.

- [ ] Extend the real CLI/API lifecycle test to begin without a friendly agent ID,
  use the returned FASE-ID, start a Sprint, expire its lease, resolve a wake, and
  assert the adapter receives that FASE-ID and its original fingerprint/session.
  The adapter must not inherit the poller's independent identity. Run to red.
- [ ] Extend adapter context with the watched fingerprint/source metadata returned
  by Core. Rehydrate it before launch; preserve legacy behavior explicitly when
  no FASE mapping exists. Never relabel old history automatically.
- [ ] Verify two agents sharing one project remain independent; reads and cache
  hits are contact only; bounded waits expire; completion/hold prevents stale
  wakes; ACK alone does not become progress; restart preserves identity and state.
- [ ] Update the input-flow document with automatic enrollment and scoped identity.
  Replace instructions that imply `tk init --agent` is required for enrollment,
  while retaining optional runtime callback/adapter setup. Document Core cache
  versus durable writes and the old-Core upgrade error. No registration command.
- [ ] Run `go test ./... -count=1`, `go vet ./internal/store ./internal/api
  ./internal/cli ./internal/guardian`, and `git diff --check`. If race tooling is
  supported, run `go test -race ./internal/store -run TestFASE -count=1`; otherwise
  record the exact toolchain limitation, not a passing race claim.
- [ ] Build into `.timekeeper/build/` and run subprocess smoke tests against an
  isolated test fixture, not another long-running live service/database. Verify
  two consecutive processes use one FASE-ID for one session and distinct IDs for
  two sessions. Confirm no identity mapping file is read on each command.
- [ ] Record exact passing/failing checks and source-versus-live limitations on
  T-10422. Complete only verified source Sprints. Leave rollout separately held
  until a safe deployment is approved; do not restart the healthy service.

## Execution review

Proposed execution: native implementation in this session, preserving the existing
dirty worktree. No subagents are needed for these sequential interfaces. Review
this plan before implementation as required by the writing-plans skill.
