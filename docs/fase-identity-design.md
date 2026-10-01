# Automatic FASE-ID identity

Status: core source implementation complete; installed-runtime rollout and verification remain pending.
Tracking: P-10053 / T-10422 / SP-10426; existing activity implementation SP-10425.

## Implementation checkpoint (2026-09-26)

Implementation tracking: T-10461 / SP-10462 (registry) / SP-10463 (bootstrap).
Source now contains the permanent reserved-key registry, month-of-use aliases,
4096-entry Core cache, automatic enrollment in both client entry points, and
masked profile JSON/HTML. Runtime-supplied model metadata is explicitly reported.
The initial checkpoint passed the full Go suite. A later run exposed `TestFASEGuardianNativeResumeKeepsOriginalIdentity`; the current full-suite run passes. This is source-only evidence, not installed-runtime proof: the local service currently returns 404 for the FASE resolve handshake, so compatible installed-runtime Guardian resume remains unverified. The rollout follow-up remains tracked in T-10461/ST-10465/SP-10475.

The current source now includes transactional claims, immutable actor snapshots,
month-filtered profile history, and Core-validated Guardian identity resume.
Focused tests are source evidence; installed-runtime rollout still requires
approved deployment and canonical verification.

## Agreed outcome

The first `timekeeper` or `tk` client invocation automatically establishes an
identity. There is no registration CLI command. Core owns a short readable ID,
for example `FASE-ID.A7K2034.9.2026`, associated with an opaque fingerprint. Later commands
reuse that identity. Different individual agent sessions must not share one
identity merely because they use the same model, workspace, or machine.

The existing work/contact/wait state machine remains the activity contract.
Identity discovery and cache hits never count as material progress.

## Permanent keys and monthly identities

The stable agent identifier returned as `agent_id` is `FASE-<key>`. Use that
value for agent-scoped CLI/API operations such as `tk guardian wake --agent`.
The separate monthly display alias returned as `fase_id` is
`FASE-ID.<key>.<month>.<year>`; it is used for profiles and historical attribution,
not as the `--agent` selector.

The key contains at least one uppercase letter and one digit. It is permanently
and uniquely coupled to one fingerprint. Core generates it and atomically
reserves it in an issued-key registry; assigned and retired keys can never be
reused for another fingerprint. Database uniqueness is authoritative; a cache
is not a substitute for it.

The same fingerprint uses the same key in every month, for example
`FASE-ID.A7K2034.9.2026` then `FASE-ID.A7K2034.10.2026`. UTC determines month and
year consistently across clients. Create a monthly alias only upon actual use
in that month. Historical events preserve their original alias. Inactive months
are not fabricated. Reset/changed fingerprints start a new key/profile series;
they cannot automatically inherit old claims. A returning unchanged fingerprint
retains its key. Inactivity never deletes identity/history or frees ownership.

## Agent profiles and historical attribution

Each full monthly FASE-ID is clickable, opening the permanent Agent Profile
filtered to that month. The profile shows a masked fingerprint, permanent key,
current and previous actually-used monthly IDs, identity scope, current work
state, claimed Tasks/Sprints, recorded work, and Guardian history.

Provider, model, model version, runtime and runtime version are optional metadata.
Unknown values remain unknown. Runtime/client-supplied values are labelled
reported, not verified. Event attribution snapshots the reported metadata and
monthly alias at the time of the event; profile updates cannot rewrite history.
Do not expose the complete fingerprint in ordinary profile/list responses.

## Execution claims and collaborative contributions

Task and Sprint execution claims are atomic and exclusive to the permanent identity.
Starting or resuming work automatically claims its Sprint and parent Task in
the same transaction. Another identity receives `409 item_already_claimed`
with the conflicting item and current owner. Same-owner claim retries are
idempotent. Different Sprints cannot bypass an existing parent Task claim.

Only the execution claimant may start, resume or complete claimed work. This is
not exclusive edit ownership: other permitted agents can add notes/evidence,
propose or create child work, and collaboratively extend information. Revision
checks protect conflicting edits; an ordered item-operation queue coordinates
only changes whose ordering matters. Independent append-only contributions do
not require a blanket execution lock. Contribution permission and execution
claim checks must remain distinct. Requests without identity cannot bypass an
existing execution claim. Claims do
not expire automatically on inactivity, waiting, service restart or month rollover.
Owner-authorized explicit release records an immutable event; release of active
work requires first placing it On Hold. Completed/cancelled Sprints retain
historical owner attribution. Parent Task release must not leave live child work
owned by another identity. Administrative forced takeover is not implicit in a
Guardian wake and must use a separately authorized, audited control path.

## Identity versus recognition

Assigning a new number requires no prior identity. Reusing the right number
requires a stable discriminator on the next invocation. A cache stores that
association but cannot create missing information about the caller.

Use runtime session metadata when exposed, starting with the verified Codex
thread/session variables and the runtime-neutral `TIMEKEEPER_SESSION_ID`
contract. A runtime adapter supplies this automatically; the agent never runs
an enrollment command or chooses its own FASE-ID. Only hash metadata, never
prompts, command bodies, credentials, or transcripts.

For an uninstrumented runtime, a process incarnation (PID plus creation time,
host scope, and bounded parent lineage) can identify an execution host. It is
not proof of a distinct logical agent when that host multiplexes agents. Expose
`identity_scope=process-session` rather than silently claiming individual-agent
recognition. A host serving multiple logical agents must propagate a distinct
session token automatically. This is an integration requirement, not a manual
registration workflow. Do not derive identity from a shared project file.

## First-use flow

1. Both executable entry points enter the same tracked CLI bootstrap before
   command dispatch, including help/version. Invalid command parsing cannot
   perform project mutations before bootstrap.
2. Bootstrap derives the fingerprint and its scope. It calls the internal
   `POST /api/v1/agent-identities/resolve` handshake; there is no matching public
   registration command.
3. Core checks its bounded in-memory fingerprint-to-identity cache. A hit
   returns the assigned ID without a registry database query.
4. A miss resolves the durable mapping, allocating a new reserved alphanumeric key
   only if none exists. A transaction and unique fingerprint constraint make
   concurrent first calls converge. Cache publication follows commit.
5. The resolved identity is attached to all ordinary CLI requests. The
   existing activity API enrolls contact and binds successful Sprint work.
   Public work-state uses a domain-separated one-way activity session token,
   not the registry fingerprint. Display-author overrides cannot replace the
   tracked caller. Identity headers are removed outside the enrolled Core origin.
6. `tk agent identity` shows the assigned monthly ID, masked fingerprint, scope, and runtime;
   it is inspection, not registration. Work/wait/idle/stop use that same ID.

If Core is offline, help and diagnostics may display local information with a
clear unresolved-identity warning; they cannot claim a Core-assigned ID.
Project reads/mutations fail with a recoverable identity/service diagnostic.
Service recovery commands remain usable so identity cannot deadlock recovery.
An old server missing the handshake is incompatible, not an excuse to execute
project work anonymously. No provisional FASE-ID is silently reassigned later.

## Cache and durability

Core/Guardian's existing long-running process owns the shared cache. Creating
a private map in each short-lived CLI would not avoid repeat registry lookups.
The cache has a mutex, a 4096-entry limit and least-recently-used eviction.
Only successful committed mappings enter it. Cache keys are scoped to the
Store instance; never share identity maps across databases/services.

SQLite remains the durable authority for restart recovery and cache misses.
Cache hits avoid identity lookup queries, not legitimate durable activity
writes. Cache eviction or restart must never allocate a replacement ID for an
existing fingerprint. IDs are opaque, immutable, and never reused. Independent
TimeKeeper services have independent key namespaces.

Registry schema: `fase_agents(agent_key TEXT PRIMARY KEY, fingerprint TEXT NOT
NULL UNIQUE, runtime TEXT NOT NULL, identity_scope TEXT NOT NULL,
metadata_json TEXT NOT NULL, created_at TEXT NOT NULL)` and permanent
`fase_reserved_keys(agent_key TEXT PRIMARY KEY)`. A separate monthly alias table
has unique alias and unique (agent_key, year, month). All allocations/reservations
are transactional. A fingerprint is exactly 64 lowercase hexadecimal characters.
Runtime and scope are validated metadata, not authentication or authorization.

## Existing identities and wake adapters

Preserve legacy rows and history. Do not rewrite an old agent label to a new
FASE-ID based solely on a supplied label. Existing legacy adapters remain
explicitly legacy until their runtime session is bound through the new path.
New tracked startup always uses the resolved FASE-ID, not a caller-chosen label.

Guardian carries the fingerprint/session identity through its trusted adapter
boundary, never the poller's own identity. It rechecks scope and current idle
state before launching. Acknowledgement proves receipt/completed adapter
boundary only; it cannot manufacture progress for state-aware agents.

## Acceptance evidence

- First ordinary command assigns a FASE-ID without init/register.
- Repeated invocations and concurrent first calls reuse the same assignment.
- Two runtime sessions in one workspace receive different assignments.
- Cache hit works without registry reads; bounded eviction and restart retain IDs.
- PID reuse cannot reuse a process-scoped fingerprint; multiplexed-host scope is
  reported honestly, not asserted to be distinct logical-agent recognition.
- Failed handshake cannot execute a requested project mutation.
- Help/doctor/service recovery remain intelligible during Core outage.
- Polling/contact do not renew progress; bounded wait expiry nudges only its owner.
- Both `tk` and the positional `timekeeper` dispatcher use the same bootstrap.
- Whole Go suite, focused race tests where supported, and executable smoke pass.
- Reserved keys are mixed letter/digit and never reassigned, including collisions.
- Monthly rollover preserves claims; profile aliases only include months of use.
- Concurrent different-agent claims cannot both acquire one Task/Sprint.
- Historical completion links and metadata remain unchanged after profile updates.
- Guardian resume preserves the watched FASE key; ACK alone never renews progress.

Deployment is separate. The current harness forbids restarting a healthy live
service as recovery. Any rollout requires an approved deployment procedure,
backup, and canonical runtime verification; source tests are not live proof.
