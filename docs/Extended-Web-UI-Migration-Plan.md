# TimeKeeper Desktop and Web UI Migration Plan — Extended

> This document extends the current actionable Web UI migration plan. It preserves the existing source-level baseline, single-Core rules, Store boundaries, parity gates, and verification requirements, while adding the architectural scaffolding now required for remote/local operation, administration, identity and access, messaging, calendar, networking, logging, Store governance, and layered backup/recovery.
>
> The original updated plan remains useful historical input. This document is the intended consolidated handoff for future migration work.

> **Directory architecture update:** Generic OIDC, SCIM, external identity records, mapping, provisioning/deprovisioning policy, and directory-provider contracts are native TimeKeeper capabilities. Microsoft Entra/Graph, traditional Active Directory/LDAP, and other vendor-specific directory integrations are optional modules/plugins.

---

## 1. Purpose

Move the human-facing TimeKeeper experience from the legacy browser dashboard toward a Wails + Svelte desktop application without replacing the existing Core, database, HTTP API, CLI, or headless agent workflows.

The desktop UI is another client of TimeKeeper. It is not a second Core, database owner, plugin runtime, Store service, authentication authority, or backup authority.

The migration must also establish the UI and API boundaries required for:

- local and remote TimeKeeper connections;
- configurable primary and fallback endpoints;
- username/password authentication that can be enabled or disabled;
- optional Microsoft, Google, GitHub, Apple, and custom OIDC login providers;
- native generic SCIM provisioning and a vendor-neutral directory-source framework;
- optional Microsoft Entra/Graph and Active Directory/LDAP provider modules;
- administrator-approved registrations;
- users, groups, roles, permissions, sessions, and API keys;
- a first-class Administration workspace;
- permission-controlled Store/module management;
- a native TimeKeeper Inbox and actionable message system;
- ntfy-backed push/email delivery;
- assignment notifications for Projects, Tasks, Subtasks, and Sprints;
- a native calendar derived from TimeKeeper project/work data;
- meeting-provider plugins such as Microsoft Teams;
- per-connection proxy, DNS, TLS, and failover settings;
- configurable application, audit, and diagnostic log locations;
- a three-layer data-protection model using Primary SQLite, SQLite shadow copies, and remote MySQL/MariaDB backup;
- remote database backup intervals of exactly 30m, 1h, 4h, 8h, 12h, or 24h.

These features must be introduced without turning the UI migration into a rewrite of the TimeKeeper Core.

---

## 2. Decisions that govern the migration

1. **One authoritative Core and SQLite owner.** `cmd/server` and `internal/store` remain the authority for durable project state and domain validation. Wails may attach to a compatible Core or supervise an explicitly configured child through the existing desktop lifecycle adapter. It must not open a second Store against the same database, choose another port/database after failure, or stop a Core it did not start.

2. **The existing versioned API is the shared command contract.** The CLI and legacy browser already use it; the Wails adapter forwards typed operations to that Core. Do not add a second application/command layer unless a concrete duplication or missing API capability is demonstrated and tested.

3. **Svelte presents validated state.** It may own view state, filtering, formatting, navigation, and interaction. It must not open SQLite, decide workflow legality, authenticate users itself, duplicate Store validation, or claim state that the backing API cannot prove.

4. **Local and remote are connection modes, not separate products.** The same UI must be able to operate against a local Core or a compatible remote Core through the same versioned TimeKeeper API contract.

5. **Remote mode must never silently fall back to a local Core.** A failed remote profile remains a failed remote profile. Local fallback behavior applies only to a local connection profile.

6. **Keep the legacy UI during parity work.** Do not remove or replace its routes/assets until the required feature matrix, supported-platform checks, rollback, and release gates pass.

7. **Keep optional services optional.** Local tracking must remain useful offline and without a Store account, social-login provider, ntfy server, remote backup server, Shadowbox provider, or external meeting provider.

8. **Authentication and authorization belong to Core/API.** Hiding a button in Svelte is not authorization. Every protected action must be enforced by Core using the current authenticated principal and permission set.

9. **Users are not synonymous with administrators.** Administrative capabilities are permission-based. Built-in roles may provide sensible defaults, but Core authorization must evaluate permissions rather than `is_admin == true`.

10. **Store installation is a privileged capability.** Store browsing may be broadly available, while installation, update, removal, activation, permission approval, and security override are separate permissions.

11. **Messaging is authoritative in TimeKeeper.** ntfy, email, webhooks, or future providers are delivery channels. They must not become the source of truth for messages, read state, workflow state, assignments, invitations, approvals, or audit history.

12. **Calendar is native TimeKeeper functionality.** External calendars and meeting systems extend or synchronize the native calendar; they do not own TimeKeeper scheduling.

13. **SQLite remains authoritative.** Local SQLite shadow copies and remote MySQL/MariaDB are recovery layers. They are not live competing writers and do not create multi-master synchronization.

14. **Feature status and deployment status are separate.** A source implementation, a successful package build, an installed desktop build, a configured remote Core, a working ntfy server, and a live Store deployment are different claims. Record evidence at the layer actually tested.

15. **The Two-Island intent uses the implemented Two-Box contract.** Core remains the protected authority; Wails/Svelte, CLI, and agents are clients, not a second database owner. Store extensions run in a separate process and may cross only the existing `timekeeper-bridge/v1` control-stdio and capability-loopback tunnels, with frozen Core-granted capabilities. Use persisted Core events/SSE rather than introducing the blueprint's in-memory event bus. The older blueprint's WASMER/gRPC and neutral-vault sketches are not implementation commitments; plugin activation stays gated until the trusted launcher, capability adapters, and security checks are verified.

---

## 3. Current source baseline

The following is a source-level snapshot, not a claim that every desktop binary is installed or running. The TimeKeeper Project task tree is authoritative for changing work status.

| Area | Source-level state | Migration implication |
|---|---|---|
| Core lifecycle | `internal/desktop` and `cmd/desktop` contain the attach/owned-child lifecycle and Wails adapter. The Core remains the only SQLite owner. | Extend this boundary; do not introduce another process or database path. Verify actual Wails runtime behavior separately from Go tests. |
| Connection discovery | `GET /api/v1/connection-info` returns stable database identity, version/API metadata, capabilities, and friendly URLs. The desktop `CoreClient` validates those fields before attach. A Go-side `ConnectionProfile` scaffold validates local/remote endpoint candidates, opaque credential references, expected instance identity, and failover compatibility. | Wails uses the fixed friendly local primary with loopback fallback and exposes read-only Test Connection diagnostics. Profile persistence/selection, remote HTTP/auth, expected-instance pinning in the UI, and automatic failover are not implemented; remote profiles never fall back locally. |
| Wails packaging | `cmd/desktop/wails.json` builds the Svelte frontend into embedded assets. | Keep Wails-specific lifecycle, native capabilities, and packaging in `cmd/desktop`; keep API/domain behavior in Core. |
| Svelte shell | Source includes Overview, Projects/Task detail, Files & History, Settings, Registration Queue, Agent Pulse, Metrics, Memory, and Plugins. Project detail separates Core events, notes, and handoff `recent_messages`. Metrics uses typed Core usage/operational summaries and keeps unpriced cost explicit. Memory search is read only and appears only when Core reports `knowledge.search`; Plugins lists catalog metadata without installation or activation actions. | Frontend check/build and Vitest pass. Browser dev-server smoke verified Agent Pulse, Trail of Thought, and Metrics against live Core data, plus Memory and Plugins safe unavailable states. This is source/dev-server evidence, not an installed Wails runtime claim. The live Memory provider and plugin store are not configured; no descriptor or Store root was changed. |
| Task/Sprint workspace | Implemented in source (`T-11497`): typed Core/Wails reads, legal lifecycle actions, refresh-after-write, and interaction tests. | Extend only through existing Core validation; do not infer status/action rules in Svelte. |
| Activity, Agent Pulse, and Trail of Thought | Implemented and verified from Core handoff snapshots plus `/api/v1/agents/work-states`; no parallel event bus or SQLite access was added. | `T-11498` and its activity/Agent Pulse/Trail work are complete after UI tests and live Core browser smoke. Keep all state read-only and Core-owned. |
| Metrics | Implemented a read-only project workspace backed by `/api/v1/projects/{projectID}/usage-summary` and `/operational-summary`; typed Wails bindings use the same Core client. | `T-11500` is complete after frontend, Go adapter, and live browser checks. Unpriced cost is explicit; planned time remains estimate + buffer + extensions, while recorded work/hold stays historical. |
| Memory Galaxy | Implemented a read-only Svelte status/search workspace over Core's `knowledge/status` and `knowledge/search` contract; search is gated on `available=true` and the advertised capability, with no promotion/edit path. | Tracker `T-11499` / `SP-11693` now shows Completed. Live Core still returns `503 provider_unavailable`; browser smoke verified only the safe unavailable state, while configured search is API/Wails-fixture tested, not live-provider tested. No descriptor was inspected or changed. |
| Plugin catalog and Settings | Read-only catalog and Settings connection diagnostics are present in source; the catalog uses Core's typed `GET /api/v1/plugins` path and offers no artifact actions. | Tracker `T-11502` / `SP-11698` shows Completed. Live Core returns `503 plugin_store_unconfigured`; browser smoke verifies the safe unavailable state. No Store root, download, install, enablement, or activation was configured. |
| Shadowbox workspace | Remaining mapped work (`T-11501`). | Current Go/Wails source has no dedicated Shadowbox workspace endpoint; only execution metadata and the extension identity are present. Do not invent a route; continue when its owning boundary exists. |
| File History and IDE | Core file-history API/service and legacy browser editor exist; the Wails/Svelte UI has a Core-backed Files & History workspace. VS Code source adds explicit history browse/restore/capture commands. | Keep one Core authority; scanning and restore are explicit, content is read from Core, and legacy browser UI remains until parity. |
| Local SQLite recovery | Source includes an integrity-check gate, A/B shadow snapshots via `VACUUM INTO`, hash/manifest verification, and explicit offline restore preserving the current DB/WAL/SHM set. | The canonical Bend2Will service now emits verified A/B snapshots under the configured E: recovery directory. After the recovery cutover, a fresh shadow-a snapshot passed both SQLite integrity and foreign-key checks and contains P-11622. The prior live DB/WAL/SHM set remains preserved. Keep restores manual and validate the chosen copy; never auto-replace a primary from a shadow. |
| Store client | `internal/storeclient` and the CLI support the optional Store protocol, authentication, catalogue/entitlement requests, package verification, and local staging. The Store server is a separate private `TimeKeeper-Store` repository. | A Svelte Store workspace does not exist yet. Treat it as a client feature, not a Store-backend migration. |
| Store activation | Staging is not execution. The trusted launcher and connector-specific capability adapters remain gated by the Store security design. | Do not expose an activation/run action until those gates have their own reviewed implementation and tests. |
| Parity and retirement | Feature-matrix, performance/accessibility, and legacy-retirement work are tracked separately (`T-11503`–`T-11505`). | Keep the legacy dashboard until these gates pass; do not equate a visual shell with feature parity. |

`docs/architecture/current-system.md` is a useful audit but includes a snapshot that predates the current Wails manifest and Project hierarchy. Refresh its source snapshot before treating it as a current-state inventory.

### 3.1 New requested capabilities are target-state work

The following additions in this extended plan are architectural targets unless the source audit proves they already exist:

- remote connection profiles;
- configurable API-key authentication for remote clients;
- users/groups/roles/permissions;
- registration and admin approval;
- social/OIDC login providers;
- TimeKeeper Inbox and actionable messages;
- ntfy/email delivery;
- native calendar;
- meeting-provider integration;
- per-profile proxy/DNS;
- configurable log paths;
- SQLite shadow recovery;
- remote MySQL/MariaDB backup.

Do not report these as implemented merely because they are specified here.

---

## 4. Target architecture

```text
Human
  ├── Legacy browser UI ── HTTP/JSON ───────────────┐
  ├── Wails + Svelte ── typed adapter ─────────────┤
  └── Browser-hosted Svelte UI ── HTTP/JSON ───────┤
                                                    │
CLI / Guardian / agents / integrations ── HTTP ────┤
                                                    ▼
                                          TimeKeeper Core API
                                                    │
                              ┌─────────────────────┼─────────────────────┐
                              │                     │                     │
                           SQLite               Services             Persisted events
                              │                     │                     │
                              │          ┌──────────┼──────────┐          │
                              │          │          │          │          │
                              │       Projects    Auth      Messages      │
                              │       Tasks       RBAC      Calendar      │
                              │       Sprints     Users     Backup        │
                              │          │          │          │          │
                              └──────────┴──────────┴──────────┴──────────┘
                                                    │
                              ┌─────────────────────┼─────────────────────┐
                              │                     │                     │
                         Store client            ntfy                Plugins
                              │                                           │
                         Cloudflare                               Teams / Calendar
                              │                                   / Storage / etc.
                       TimeKeeper-Store
```

### 4.1 Wails lifecycle boundary

```text
Wails lifecycle adapter
  ├── attach to existing compatible local Core
  ├── own one explicitly configured local Core child
  └── connect to configured remote Core without starting a local child
```

Wails-specific lifecycle and native capabilities remain adapter concerns. Domain behavior remains in Core.

### 4.2 API boundary

The Wails typed adapter is transport/lifecycle glue. It must not add its own business rules.

Browser development may use the existing same-origin API path. Wails uses typed Go bindings backed by the same Core API contract. Remote Wails mode uses the remote API contract.

If a feature needs data the API does not expose, first prove the gap and add the smallest Core endpoint/DTO. Do not reach around Core into SQLite.

### 4.3 Event boundary

The existing persisted event/SSE interface is the first choice for durable activity updates. Use bounded refresh where live delivery is not needed.

A Wails event-forwarding adapter may be added if a concrete view needs streaming that the WebView cannot consume. That is transport glue over persisted Core events, not a reason to create a second event bus.

Do not add a new in-memory event bus pre-emptively.

---

# 5. Application information architecture

The refactored UI should distinguish ordinary workspaces, user settings, and instance administration.

```text
TimeKeeper
├── Dashboard
├── Projects
├── Files & History
├── Tasks
├── Calendar
├── Messages
├── Agents
├── Memory
├── Metrics
├── Store
│
├── Settings
│   ├── General
│   ├── Connection
│   ├── Notifications
│   ├── Appearance
│   └── Personal preferences
│
└── Administration
    ├── Users
    ├── Groups
    ├── Roles & Permissions
    ├── Registrations
    ├── Authentication
    ├── Sessions
    ├── API Keys
    ├── Audit Log
    ├── Network
    ├── Logging & Diagnostics
    ├── Backup & Recovery
    ├── Store & Extensions
    ├── Integrations
    ├── Storage
    ├── System
    └── Security
```

**Files & History** is a Core-backed workspace over explicitly configured per-Project source roots and immutable file versions. Source scanning is an explicit action; restore appends a new branch through Core rather than rewinding or deleting history.

**Settings** means personal or connection preferences.

**Administration** means instance governance, security, users, infrastructure, privileged Store/module actions, and system-wide policy.

Administration visibility must be permission-driven.

---

# 6. Connection profiles and local/remote operation

## 6.1 Connection profile model

The UI must not assume one anonymous local server.

The target connection abstraction should include:

```text
ConnectionProfile
├── id
├── name
├── mode: local | remote
├── primary_endpoint
├── secondary_endpoint
├── automatic_failover
├── expected_instance_id
├── authentication mode/reference
├── TLS policy
├── proxy policy
├── DNS policy
├── cached capability metadata
├── last successful endpoint
└── diagnostics metadata
```

The current Go source contains a partial profile contract for ID/name, local-or-remote mode, primary/secondary endpoints, automatic failover, expected instance ID, auth mode, and an opaque credential reference. It validates remote endpoint isolation and same-instance/API/capability compatibility, and its JSON decoder rejects unknown fields such as a raw `api_key`. This is validation scaffolding only: profiles are not persisted or selectable in the UI, and remote HTTP/authentication is not implemented.

Frontend application state should be built around at least:

```text
currentInstance
currentConnection
currentIdentity
currentPermissions
currentCapabilities
currentWorkspace
```

## 6.2 Local connection

The preferred local endpoint is `http://timekeeper.local`; source-level Wails startup maps that exact hostname to loopback, bypasses system proxy routing, and falls back to `http://127.0.0.1:1618` only when the friendly endpoint is unreachable. An incompatible Core is reported as incompatible rather than hidden by fallback. The fallback is local-profile behavior only; it is not available to a remote profile.

Settings now includes an explicit, read-only **Test connection** action. Wails reports the selected Core endpoint, whether the loopback fallback was used, and Core identity/API/capabilities; browser builds report their browser-facing API origin and explicitly do not claim to control native proxy/DNS routing. Per-profile selection, authentication state, DNS/TLS/proxy diagnostics, and remote endpoint checks remain unimplemented.

## 6.3 Remote connection

Remote profiles support:

```text
Primary endpoint:
https://tk.example.com

Secondary endpoint:
https://tk-alt.example.com
```

Resolution:

```text
Primary
  ↓ unavailable
Secondary
  ↓ unavailable
Connection failed
```

A remote failure must never silently connect to the local connection profile.

## 6.4 Instance identity and compatibility

Core exposes stable instance and compatibility metadata at `GET /api/v1/connection-info`. `instance_id` is generated once and persisted with the SQLite database, so a database backup/restore preserves the logical identity while a newly initialized database receives a different ID. The endpoint is discovery metadata only; it does not enable remote connections or failover.

Response shape:

```json
{
  "service": "TimeKeeper",
  "instance_id": "tk_instance_<32 lowercase hex characters>",
  "version": "<build version>",
  "api_version": "v1",
  "capabilities": [
    "execution_batches.validate",
    "project.events",
    "projects.execution_tree",
    "projects.list",
    "tasks.batch.validate",
    "tasks.lifecycle"
  ],
  "friendly_urls": []
}
```

Build fields (`commit`, `build_date`, `go_version`) are included when available. Capabilities are sorted; optional capabilities are included only when their backing service is configured. `friendly_urls` remains available to existing clients and is encoded as an empty array when no URLs are advertised. The current desktop attach probe requires a syntactically valid instance ID, matching service/build metadata, API version `v1`, and the `projects.list` plus `tasks.lifecycle` capabilities; it reports the specific compatibility gap instead of attaching to an older/mismatched Core. The local desktop client accepts only the exact `timekeeper.local` alias or numeric loopback; it uses `timekeeper.local` first and the numeric fallback only when the friendly endpoint is unreachable. Settings now exposes a read-only Test Connection action; remote profiles, expected-instance binding, authentication, failover pairing, and advanced network diagnostics remain unfinished.

Before enabling automatic remote failover, verify:

```text
primary.instance_id == secondary.instance_id
```

If the IDs differ, warn and do not treat the endpoints as an automatic failover pair.

Capability discovery is preferred over assuming all compatible versions expose identical optional functionality.

## 6.5 API-key remote access

Remote TimeKeeper must support machine/client authentication similar in shape to common AI APIs:

```text
Base URL:
https://tk.example.com/v1

Authorization:
Bearer <TimeKeeper API key>
```

This is appropriate for:

- `tk` CLI;
- Hermes/Cipher agents;
- service integrations;
- automation;
- desktop remote profiles.

Raw API keys must only be shown at creation/reset and must not be stored in Svelte `localStorage`.

---

# 7. Authentication, identities, users, groups, and permissions

## 7.1 Authentication toggle

Administration → Authentication must expose an explicit authentication requirement toggle:

```text
Require authentication  [ON/OFF]
```

### Authentication OFF

Appropriate for an explicitly trusted single-user local instance.

The Core may represent access as an implicit local Owner principal, but authorization behavior must still be internally coherent.

### Authentication ON

Protected UI and API access require an authenticated principal.

Username/password login is the baseline method.

The Svelte login screen is a client of the Core authentication API. Svelte does not validate passwords itself.

## 7.2 Initial administrator bootstrap

If authentication is enabled and no privileged user exists:

```text
Enable authentication
  ↓
No Owner exists
  ↓
Create initial Owner
  ↓
Authentication becomes active
```

The transition must be atomic enough that an administrator cannot accidentally enable authentication and permanently lock out the instance.

Keep an emergency local Owner/recovery path available according to the security design, particularly when external identity providers are enabled.

## 7.3 Identity types

Authentication and API access should support first-class principals:

```text
Human
Agent
Service
API Client
```

Audit attribution must identify the actual principal rather than treating every token as an anonymous user.

## 7.4 Users

A user record should support:

```text
username
display name
verified email(s)
enabled/disabled state
registration state
group membership
role assignment
linked identities
created_at
created_by
last_login
password_changed_at
```

Passwords are stored as secure password hashes, never plaintext.

## 7.5 Groups, roles, and permissions

Use RBAC-style permissions.

```text
User
  ↓
Groups / direct role assignment
  ↓
Roles
  ↓
Permissions
```

Built-in roles may include:

```text
Owner
Admin
Manager
Member
Viewer
Agent
```

Custom roles must be supported.

Representative permissions:

```text
projects.read
projects.create
projects.edit
projects.delete

tasks.read
tasks.create
tasks.edit
tasks.start
tasks.stop
tasks.complete
tasks.delete

sprints.read
sprints.manage

calendar.read
calendar.manage

messages.read
messages.send

totb.read
totb.write

agents.read
agents.control

memory.read
memory.write

metrics.read

users.read
users.create
users.edit
users.disable

groups.read
groups.manage

roles.read
roles.manage

registrations.manage

directory.read
directory.configure
directory.sync
directory.mapping.manage
directory.provisioning.manage
identity.providers.manage
scim.manage

apikeys.create
apikeys.revoke

settings.read
settings.manage

network.manage
logging.manage
backup.manage

store.read
store.request_install
store.install
store.update
store.remove

plugins.enable
plugins.disable
plugins.configure
plugins.permissions.review
plugins.permissions.approve
plugins.security.override

admin.access
audit.read
system.manage
```

Do not hardcode protected actions around role names.

## 7.6 Sessions

Username/password or social/OIDC login creates a Core-managed session.

Session state should support:

```text
session_id
principal_id
created_at
last_seen_at
expires_at
revoked_at
device/client metadata
network metadata where appropriate
```

Administrators with sufficient permissions can inspect and revoke sessions.

---

# 8. Registration and administrator approval

Registration must be independently configurable from authentication.

```text
Allow registration        [ON/OFF]
Require admin approval     [ON/OFF]
```

Recommended shared/remote default:

```text
Authentication      ON
Registration        ON
Admin approval      ON
Auto approval       OFF
```

Registration state model:

```text
pending_verification
pending_approval
active
suspended
rejected
disabled
```

Flow:

```text
User registers
  ↓
Identity/email verification
  ↓
Account status = pending_approval
  ↓
Administrator review
  ├── approve
  ├── reject
  └── request correction / leave pending
```

A pending account does not receive normal TimeKeeper application access. It may receive only a narrow status view.

Users may request access/groups where useful, but may not self-grant roles or permissions.

The Administration workspace includes:

```text
Registrations
├── Pending
├── Approved
├── Rejected
└── Suspended
```

---

# 9. Identity providers, OIDC, SCIM, and directory integrations

TimeKeeper should hardcode the **generic identity/directory framework and open protocols**, not individual enterprise vendors.

The intended split is:

```text
TimeKeeper Core
├── Local users
├── Users / Groups / Roles / Permissions
├── External identities
├── Generic OIDC
├── Generic SCIM 2.0 provisioning
├── Directory-source/provider contract
├── Group/role mapping
├── Provisioning/deprovisioning policy
├── Sync state/history
└── Audit

Optional provider modules
├── Microsoft Enterprise
│   ├── Entra ID / Microsoft Graph
│   ├── Microsoft-specific directory sync
│   ├── Teams
│   ├── Outlook Calendar
│   └── optional OneDrive capabilities
├── Active Directory / LDAP
│   └── LDAP / LDAPS
├── Google Workspace
├── Okta
├── Keycloak
└── future directory providers
```

This keeps identity and authorization first-class while preventing Microsoft-specific dependencies and tenant behavior from becoming mandatory Core functionality.

## 9.1 Login providers

Authentication providers are optional and individually enableable.

```text
Username + Password   [ON/OFF]
Microsoft             [ON/OFF]
Google                [ON/OFF]
GitHub                [ON/OFF]
Apple                 [ON/OFF]
Custom OIDC           [ON/OFF]
```

Generic OIDC support belongs in Core. Provider presets may simplify Microsoft/Google configuration, but they must use the same underlying identity boundary rather than creating separate authorization systems.

Provider configuration must remain Core/server-side and may include:

```text
issuer / tenant
client ID
client secret reference
allowed domains
scopes
callback configuration
```

Secrets must not be exposed to the WebView or stored in browser-local state.

A social/OIDC provider proves identity; it does not automatically grant TimeKeeper access.

Flow:

```text
Microsoft / Google / OIDC verifies identity
  ↓
TimeKeeper resolves or creates pending identity
  ↓
Registration policy
  ↓
Admin approval if required
  ↓
TimeKeeper groups / roles / permissions
  ↓
Session
```

Linked identities may allow one TK user to authenticate through multiple enabled providers.

## 9.2 Native directory framework

TimeKeeper Core should expose a vendor-neutral directory/provider contract rather than hardcoding Microsoft Graph or LDAP behavior into users/RBAC.

Conceptual provider capabilities:

```text
DirectoryProvider
├── TestConnection()
├── GetStatus()
├── InitialSync()
├── GetChanges()
├── SyncUsers()
├── SyncGroups()
├── SyncMemberships()
└── MapAttributes()
```

The exact Go interface may differ and should follow existing project conventions; this list defines capability, not required method names.

The native framework should support:

```text
Directory sources
User provisioning
Group provisioning
Membership synchronization
Attribute mapping
Group → TK group/role mapping
Provisioning policy
Deprovisioning policy
Sync checkpoints
Sync history
Error/status reporting
Audit attribution
```

TimeKeeper remains authoritative for its own roles and permissions.

External directories may supply identities, users, groups, and memberships. They do not bypass TimeKeeper authorization.

## 9.3 Generic SCIM 2.0

SCIM support belongs in Core as an open provisioning protocol.

Conceptual architecture:

```text
Entra / Okta / other SCIM client
              ↓
       TimeKeeper SCIM API
              ↓
      TK Users / Groups
              ↓
       Roles / Permissions
```

SCIM may create, update, disable, and maintain memberships according to configured policy.

SCIM does not synchronize passwords.

SCIM-created users may be:

```text
○ imported as Pending
○ imported as Active
○ accepted only from explicitly mapped groups
```

The administrator chooses the provisioning policy.

## 9.4 Microsoft Entra / Graph module

Microsoft-specific directory ingestion should be implemented as an optional module/plugin, not required Core code.

The Microsoft module may support:

```text
Entra ID users
Entra groups
group memberships
selected profile attributes
incremental/delta synchronization
Microsoft-specific tenant configuration
Microsoft group mapping
```

Example mapping:

```text
External group             TimeKeeper mapping

TK-Developers          →   Developers
TK-Managers            →   Project Managers
TK-ReadOnly            →   Viewer
```

Mappings must be explicit. Do not infer administrative privilege from an external group name.

The module must not synchronize Microsoft passwords.

## 9.5 Active Directory / LDAP module

Traditional Active Directory integration should be provided through an optional LDAP/LDAPS connector.

Conceptual flow:

```text
Windows Active Directory
        ↓ LDAP / LDAPS
TimeKeeper directory module
        ↓
TK Users / Groups / Memberships
        ↓
TK Roles / Permissions
```

Prefer LDAPS or another reviewed secure transport for production deployment.

The LDAP/AD module must not become required for normal local authentication.

## 9.6 Provisioning and deprovisioning

Directory removal must not erase TimeKeeper history.

Recommended behavior:

```text
External user removed/disabled
        ↓
TimeKeeper account disabled
        ↓
active sessions revoked
        ↓
external memberships removed/reconciled
        ↓
historical attribution retained
```

Historical records such as tasks, messages, ToTb posts, meetings, audit records, and project actions continue to reference the disabled identity.

## 9.7 Directory administration

Administration should include:

```text
Directory & Identity
├── Directory Sources
├── OIDC Providers
├── SCIM Provisioning
├── Attribute Mapping
├── Group Mapping
├── Provisioning Rules
├── Deprovisioning Rules
├── Sync Schedule / Sync Now
├── Sync History
└── Provider Diagnostics
```

Suggested permissions:

```text
directory.read
directory.configure
directory.sync
directory.mapping.manage
directory.provisioning.manage
identity.providers.manage
scim.manage
```

A delegated `Directory Administrator` role can manage enterprise identity without automatically gaining unrelated system, Store, backup, or plugin privileges.

---

# 10. Administration workspace

Administration is a first-class product surface.

```text
Administration
├── Users
├── Groups
├── Roles & Permissions
├── Registrations
├── Authentication Providers
├── Directory & Identity
├── Sessions
├── API Keys
├── Audit Log
├── Network
├── Logging & Diagnostics
├── Backup & Recovery
├── Store & Extensions
├── Integrations
├── Storage
├── System
└── Security
```

Examples of delegated administrative roles:

```text
User Administrator
  ✓ users.manage
  ✓ groups.manage
  ✓ registrations.manage
  ✗ network.manage
  ✗ system.manage

Extension Manager
  ✓ store.install
  ✓ store.update
  ✓ plugins.enable
  ✓ plugins.configure
  ✗ users.manage
  ✗ auth.manage
```

The Core must enforce every privileged operation.

---

# 11. Internal TimeKeeper messaging and Inbox

## 11.1 Core ownership

TimeKeeper owns the authoritative message record.

The Inbox is not a thin ntfy client.

```text
Core event / user action
  ↓
TimeKeeper Message
  ↓
Persisted in SQLite
  ↓
Delivery policy
  ├── in-app
  ├── ntfy push
  ├── email through notification backend
  └── future provider/plugin
```

In-app messages remain available even if every external delivery provider is disabled.

## 11.2 Message model

Conceptual fields:

```text
message_id
type
sender_principal_id
recipient_user_id
recipient_group_id
subject
body
priority
entity_type
entity_id
state
actions[]
created_at
read_at
acknowledged_at
expires_at
```

Representative states:

```text
unread
read
acknowledged
accepted
declined
completed
expired
cancelled
```

Types may define different legal state transitions.

## 11.3 Actionable messages

The message subsystem must support actions, not only text.

Examples:

```text
Task T-1042 assigned to you
[Open Task] [Acknowledge]

Project invitation
[Accept] [Decline] [View Project]

Collaboration request
[Approve] [Reject] [Change Role]

Password expires soon
[Change Password] [Remind Me Later]
```

Core validates every message action.

## 11.4 Built-in use cases

The same message infrastructure should support:

- Task assignment;
- Subtask assignment;
- Sprint assignment;
- Project assignment/invite;
- reassignment;
- collaboration requests;
- project join requests;
- user-registration approval;
- password-change reminders;
- password expiry;
- security warnings;
- approval requests;
- deadline reminders;
- task failure;
- backup failure;
- plugin/module install requests;
- Store/security notices;
- system warnings;
- agent handoffs;
- mentions;
- future plugin-defined message types.

## 11.5 Assignment integration

Durable work entities may reference users/groups according to existing domain constraints.

Assignment creates a message transactionally or through an auditable persisted workflow so that the assignment and notification cannot silently diverge.

---

# 12. ntfy and notification delivery

ntfy is a notification transport, not the TimeKeeper message database.

Administration may configure the instance ntfy backend.

Users may configure their own allowed delivery preferences.

```text
Notifications
├── In-app               always available
├── ntfy push            ON/OFF
├── Email                ON/OFF
└── Event preferences
```

Example user preferences:

```text
Job assigned             ON
Job reassigned           ON
Deadline approaching     ON
Mentions                 ON
Approval requests        ON
Task completed           optional
System messages          optional
```

Instance policy may mark some in-app/security notifications as mandatory.

Email preferences must be decided by TimeKeeper policy before a notification is submitted to ntfy/email delivery.

ntfy topics/credentials must be protected and should not rely on predictable public topic names without access control.

Notification-delivery failure must not delete or invalidate the authoritative TimeKeeper message.

---

# 13. Native TimeKeeper calendar

Calendar is Core functionality because Project, Task, Subtask, Sprint, assignment, milestone, reminder, and date/time data already belong to TimeKeeper.

## 13.1 Calendar model

Conceptual native event:

```text
CalendarEvent
├── id
├── type
├── title
├── description
├── project_id
├── task_id
├── subtask_id
├── sprint_id
├── owner_user_id
├── participants[]
├── start_at
├── end_at
├── timezone
├── all_day
├── recurrence
├── reminder_rules[]
├── status
├── visibility
├── created_by
├── provider
└── provider_reference
```

## 13.2 Derived vs explicit events

Avoid duplicating dates already authoritative elsewhere.

### Derived events

Generated/projected from:

- project dates;
- Task start/due dates;
- Subtask dates;
- Sprint start/end;
- milestones;
- assigned-work deadlines.

### Explicit calendar events

Persisted independently:

- meetings;
- reminders;
- appointments;
- planning sessions;
- personal/team events.

## 13.3 Calendar views

The UI should support at least:

```text
Today
Week
Month
Agenda
```

Filters/scopes:

```text
My Calendar
Team
Project
All

Projects
Tasks
Sprints
Meetings
Milestones
Personal
Reminders
```

Project workspaces may expose a project-scoped Calendar tab.

## 13.4 Timezone rule

Persist absolute timestamps in a timezone-safe form, preferably UTC where appropriate, and render using the user's configured timezone.

Do not store ambiguous local-time strings as the canonical timestamp.

---

# 14. Meetings and external calendar providers

TimeKeeper owns the meeting/calendar record. External providers enrich or synchronize it.

A meeting-provider plugin may provide:

- Microsoft Teams;
- Google Meet;
- Zoom;
- Jitsi;
- future providers.

A Microsoft integration may also expose Outlook calendar synchronization where its permission model allows.

Provider-specific values belong on the native event:

```text
provider = microsoft-teams
provider_reference = ...
join_url = ...
```

Removing the provider must not erase the TimeKeeper meeting history.

Meeting creation can integrate with Messaging:

```text
Create meeting
  ↓
Calendar event
  ↓
Invite participants
  ↓
TK Inbox
  ↓
ntfy/email according to policy
```

Future transcript/meeting-summary plugins may propose decisions or Tasks, but task creation from extracted meeting content should be separately authorized/approved according to policy.

---

# 15. Network, proxy, DNS, TLS, and failover

## 15.1 Per-connection network policy

Network settings should be associated with a connection profile where practical.

```text
Network
├── Proxy
├── DNS
├── TLS
└── Failover
```

## 15.2 Proxy

Target modes:

```text
System/default
Direct / no proxy
HTTP
HTTPS
SOCKS5
Environment configuration
```

Possible fields:

```text
host
port
username reference
password/secret reference
proxy DNS toggle
bypass list
```

Credential secrets remain outside Svelte/localStorage.

## 15.3 DNS

Target modes:

```text
System DNS
Custom resolver
DNS-over-HTTPS
DNS-over-TLS
```

Optional policy:

```text
primary resolver
secondary resolver
fallback to system DNS
cache policy
IPv4/IPv6 preference
```

Local `timekeeper.local` resolution must preserve the local resolution path before falling back to `127.0.0.1:1618`.

## 15.4 Browser limitation

A normal browser-hosted UI cannot reliably override the browser/OS proxy or DNS stack.

Advanced proxy/DNS controls therefore primarily govern Wails/native Core networking and other Core-originated outbound requests.

Browser UI must show these controls as unsupported/read-only where the browser cannot honor them.

## 15.5 Diagnostics

Connection diagnostics should be able to report independently:

```text
Endpoint reachability
DNS resolution
Proxy path
TLS validation
API compatibility
Instance identity
Authentication
Latency
Selected endpoint
```

---

# 16. Logging and diagnostics

## 16.1 Configurable log location

Administration → Logging & Diagnostics should allow:

```text
Default application location
Custom folder
Project-relative folder where safe
System log/journal where supported
```

Example defaults may be platform-specific, but the implementation must not hardcode a user-specific path.

Changing a log path must never prevent TimeKeeper from starting.

If a configured destination is unavailable:

```text
configured path fails
  ↓
fallback to safe local default
  ↓
record warning
  ↓
continue running
```

## 16.2 Log classes

Keep distinct logical outputs where useful:

```text
core
api
desktop
agents
shadowbox
plugins
store
network
database
audit
crashes
```

Not every deployment must physically split these into separate files; configuration may choose aggregation.

## 16.3 Log levels

Allow per-component levels such as:

```text
ERROR
WARN
INFO
DEBUG
TRACE
```

with safe defaults.

## 16.4 Rotation and retention

Support bounded rotation and retention policies.

Possible rotation modes:

```text
size
daily
size + daily
```

Rotated logs may be compressed.

Audit-log retention must be independently configurable from ordinary debug/runtime logs.

## 16.5 Diagnostic bundle

Provide:

```text
Open Log Folder
Create Diagnostic Bundle
```

A diagnostic bundle may include:

- selected logs;
- Core/desktop/API version;
- capability metadata;
- sanitized configuration;
- recent errors;
- integrity/health summaries.

It must exclude passwords, raw API keys, OAuth secrets, session tokens, signing secrets, and other credentials.

---

# 17. Store and extension governance

The Store remains optional distribution/entitlement infrastructure. It is not required for ordinary local tracking.

The Store UI may surface connection/configuration, account/device state, catalogue, entitlements, package verification, and staging through the existing Go-side Store client boundary.

The private Store backend remains a separate service/repository.

Choosing a different Store-portal frontend framework does not authorize a Store backend rewrite.

## 17.1 Permission-controlled module management

Do not hardcode Store installation to the `Admin` role.

Use permissions:

```text
store.read
store.request_install
store.install
store.update
store.remove

plugins.enable
plugins.disable
plugins.configure

plugins.permissions.review
plugins.permissions.approve
plugins.security.override
```

Example:

```text
Member
  ✓ store.read
  ✓ store.request_install
  ✗ store.install

Extension Manager
  ✓ store.read
  ✓ store.install
  ✓ store.update
  ✓ plugins.enable
  ✓ plugins.configure
  ✗ users.manage
```

## 17.2 Installation request workflow

Users without installation permission may request a module:

```text
User selects module
  ↓
Request install
  ↓
TK actionable message / approval request
  ↓
Authorized principal reviews
  ├── package/version
  ├── publisher
  ├── signature
  ├── security result
  └── requested permissions
  ↓
Approve / Reject
```

## 17.3 Staging remains separate from activation

```text
Download
  ↓
Verify
  ↓
Stage
  ↓
Review permissions/security
  ↓
Activate through separately approved trusted launcher
```

Do not expose activation/run until trusted launcher, permission policy, isolation, and connector-adapter gates are actually implemented.

---

# 18. Backup and recovery architecture

Backup/recovery is a native Core responsibility and must not be implemented as UI-side file copying.

The target is a three-layer protection model:

```text
Layer 1 — Live authoritative SQLite
          timekeeper.db
              │
              ▼
Layer 2 — Local SQLite shadow recovery
          shadow-a.db / shadow-b.db
              │
              ▼
Layer 3 — Remote MySQL/MariaDB recovery backup
```

SQLite remains the only live authoritative database.

There is no automatic MySQL/MariaDB → SQLite live synchronization and no multi-writer behavior.

---

# 19. Layer 1 — Primary SQLite

Primary SQLite remains the live database.

Normal Core reads/writes use only the authoritative SQLite Store.

Existing database ownership, validation, schema, and migration rules remain unchanged unless a separately reviewed feature requires a schema change.

Backup features must not turn a presentation-layer migration into a database redesign.

---

# 20. Layer 2 — SQLite shadow copy

## 20.1 Purpose

Provide fast local recovery from:

- accidental file corruption;
- failed update/migration;
- local database damage;
- operator error where the current primary becomes unusable.

## 20.2 Snapshot behavior

Do not naïvely copy an open SQLite file.

Use SQLite-supported consistent backup/snapshot mechanisms.

Recommended structure:

```text
data/
└── timekeeper.db

recovery/
├── shadow-a.db
└── shadow-b.db
```

Use A/B rotation so a failed snapshot cannot destroy the only known-good shadow.

Conceptual cycle:

```text
Primary
  ↓ consistent snapshot
Shadow A
  ↓ integrity verify
mark A healthy

next cycle:
Primary
  ↓
Shadow B
  ↓ integrity verify
mark B healthy
```

The Core implementation uses `Store.BackupTo` (`VACUUM INTO`) rather than a raw database/WAL copy. Before the operation it requires `PRAGMA integrity_check` to return exactly `ok`; it then verifies the staged snapshot read-only and computes a SHA-256 before promoting the inactive slot. The scheduled A/B worker defaults to five minutes, accepts `-shadow-copy-dir` and `-shadow-copy-interval`, and skips without rotating either slot when the primary is unhealthy. Source implementation does not imply an already-updated service binary; the installer must pass the flags and runtime evidence must show verified slot/path/hash log entries.

## 20.3 Shadow location

Allow a configurable shadow directory.

Recommend a different physical volume where available.

UI diagnostics may warn when primary and shadow are on the same physical drive.

## 20.4 Shadow schedule

The shadow layer may use a faster cadence than remote backup.

Initial UI scaffolding may provide radio-button presets such as:

```text
1 minute
5 minutes
15 minutes
30 minutes
```

The final cadence set should be confirmed against measured database size/write cost before release.

A `Create Shadow Now` action is useful for administration and pre-maintenance checkpoints.

## 20.5 Recovery

Recovery must verify the candidate shadow before replacing the authoritative database.

The source implementation provides an offline `-restore-shadow-slot shadow-a.db -confirm-shadow-restore` (or B-slot) maintenance mode. It refuses when the database instance lock or configured loopback listener is busy (default `127.0.0.1:1618`), checks the candidate before modifying the primary, preserves the old database plus WAL/SHM in a timestamped recovery directory, stages the candidate beside the primary, verifies the installed file, and exits without restarting Core. It does not choose a slot automatically and does not turn a health-check failure into an automatic rollback/restore action.

Do not overwrite the current primary until the restore candidate passes the required integrity checks.

---

# 21. Layer 3 — Remote MySQL/MariaDB backup

## 21.1 Purpose

MySQL/MariaDB is an off-machine disaster-recovery layer to reduce data-loss exposure.

It is not a second TimeKeeper Core database and is not used for normal live queries.

## 21.2 Configuration

Administration → Backup & Recovery:

```text
Remote SQL Backup
├── Enabled
├── Engine: MySQL | MariaDB
├── Host
├── Port
├── Database
├── Username
├── Password/secret reference
├── TLS
├── Backup interval
├── Run missed backup on startup
├── Retention/history policy
├── Verify after backup
├── Notify on failure
├── Test Connection
├── Backup Now
└── Restore
```

Credentials remain outside Svelte-local storage.

## 21.3 Allowed intervals

Use a radio-button group with exactly one active interval:

```text
( ) 30 minutes
(●) 1 hour       Recommended default
( ) 4 hours
( ) 8 hours
( ) 12 hours
( ) 24 hours
```

These are the supported remote backup intervals:

```text
30m
1h
4h
8h
12h
24h
```

Do not expose arbitrary interval input unless a future requirement justifies it.

The selected interval represents the approximate maximum unsent-backup window during normal operation.

## 21.4 Missed backups

Option:

```text
Run missed backup on startup [ON/OFF]
```

If the elapsed time since the last healthy backup exceeds the selected interval, the Core may run a catch-up backup after startup once the system is ready.

## 21.5 Backup semantics

Prefer consistent checkpoint/history behavior over destructive mirroring.

The backup process should preserve enough information to recover from accidental deletion or corruption that occurred before the latest run.

The implementation may use:

- full checkpoints;
- incremental change journals;
- periodic full + incremental sequences;
- another reviewed mechanism that supports deterministic restore.

Do not simply make the remote backup indistinguishable from the latest damaged state without recovery history.

## 21.6 Failure behavior

Remote backup failure must never stop normal TimeKeeper operation.

```text
Remote DB unavailable
  ↓
mark backup failed
  ↓
preserve pending/next work
  ↓
continue using SQLite
  ↓
create TK warning message
  ↓
ntfy/email according to policy
  ↓
retry on next eligible cycle / manual retry
```

## 21.7 Verification

A backup is not marked healthy solely because a network write returned success.

Record enough verification metadata to prove the recovery point is structurally usable according to the selected backup mechanism.

---

# 22. Backup and recovery UI

Suggested Administration surface:

```text
Backup & Recovery

Primary SQLite
  Status            Healthy
  Path              ...
  Integrity         Verified

Local Shadow
  Enabled           Yes
  Active generation A
  Last snapshot     ...
  Last verification Healthy
  Location          ...
  [Create Shadow Now]

Remote SQL Backup
  Enabled           Yes
  Engine            MariaDB
  Last backup       ...
  Next backup       ...
  Interval          1 hour
  Last verification Healthy
  Pending           ...
  [Backup Now]
  [Restore]
```

Restore should present known recovery points and their verification status.

Destructive restore operations require privileged permissions and explicit confirmation.

---

# 23. Audit trail

Once TimeKeeper supports multiple humans, agents, API keys, registration approvals, module installation, remote access, and restore operations, audit attribution is essential.

Representative audit events:

```text
AUTH.LOGIN
AUTH.LOGIN_FAILED
AUTH.PROVIDER_CHANGE
USER.CREATE
USER.DISABLE
REGISTRATION.APPROVE
REGISTRATION.REJECT
ROLE.UPDATE
GROUP.UPDATE
APIKEY.CREATE
APIKEY.REVOKE

TASK.ASSIGN
TASK.START
TASK.COMPLETE

MESSAGE.ACTION
PROJECT.INVITE_ACCEPT

STORE.INSTALL_REQUEST
STORE.INSTALL
PLUGIN.ENABLE
PLUGIN.PERMISSION_APPROVE

NETWORK.CONFIG_CHANGE
LOGGING.CONFIG_CHANGE

BACKUP.RUN
BACKUP.FAIL
BACKUP.RESTORE
SHADOW.RESTORE
```

Audit data is distinct from ordinary runtime/debug logs.

Permissions such as `audit.read` control access.

---

# 24. Phased delivery and acceptance gates

The original migration gates remain valid. New functionality is inserted as additional gates rather than renumbering existing source-linked work unnecessarily.

---

## Gate 0 — Boundary and feature inventory

Maintain a feature matrix with one row per human-visible capability and columns for:

- legacy UI;
- Core API/provider;
- Svelte UI;
- CLI/headless behavior;
- tests/browser evidence;
- desktop package;
- deployment status.

Add rows for all new target areas in this plan:

- connection profiles;
- remote mode;
- identity/authentication;
- registrations;
- social login;
- administration;
- messages;
- ntfy;
- calendar;
- meetings;
- proxy/DNS;
- logging;
- Store governance;
- SQLite shadow;
- remote SQL backup.

Use real route/DTO shapes and current source. Distinguish unavailable optional services from empty data.

**Pass when:** every capability in scope has an identified authoritative source and a testable acceptance statement. No broad Go package move or business-rule extraction is part of this gate.

---

## Gate 1 — Wails/Core lifecycle and desktop shell

Keep the existing attach-or-owned-child lifecycle.

Require:

- numeric loopback where applicable;
- compatible health/version;
- explicit absolute executable/database/UI paths before launching a child;
- single-instance protection;
- bounded readiness;
- shutdown of only an owned child;
- preservation of the Core API for `tk`, Guardian, agents, and integrations.

Add remote lifecycle behavior:

```text
Local profile
  → attach / owned child

Remote profile
  → never launch or silently fall back to local Core
  → connect only to profile endpoints
```

**Pass when:** focused lifecycle tests cover attach, incompatible/occupied Core, missing paths, timeout, early child exit, owned-vs-attached shutdown, remote connection failure, and remote-no-local-fallback; a supported-platform Wails runtime smoke proves attach and owned-child cleanup without creating a second database.

A Go unit test alone is not runtime proof.

---

## Gate 1A — Connection profiles, remote endpoints, and capability discovery

Implement the connection-profile scaffold before the UI becomes dependent on a single anonymous local endpoint.

Verified source progress (partial gate only): Core connection discovery exposes persistent instance identity, API/build metadata, and capabilities; desktop attach enforces compatibility and the Wails local profile uses the friendly primary with numeric-loopback fallback. Settings includes an explicit read-only Test Connection result. A Go-side profile scaffold validates local/remote candidates, opaque credential references, and expected-instance/API/capability compatibility; it is not persisted or used for remote HTTP. Profile selection, remote auth, failover execution, and keychain integration remain outstanding.

Requirements:

- fixed local primary `http://timekeeper.local`;
- fixed local fallback as documented in §15.3;
- remote primary endpoint;
- optional remote secondary endpoint;
- optional automatic failover;
- same-instance verification for failover;
- API version/capability discovery;
- Test Connection diagnostics;
- no local fallback from remote mode;
- secure API-key reference support.

**Pass when:** local fallback is deterministic; remote profiles cannot silently attach locally; mismatched remote instance IDs are rejected as automatic failover pairs; capability/version incompatibility is displayed honestly; API-key secrets are not exposed to Svelte persistence.

---

## Gate 1B — Network, proxy, DNS, TLS, and diagnostics scaffold

Implement the native networking configuration boundary.

Requirements:

- system/direct proxy modes;
- HTTP/HTTPS/SOCKS5 where supported;
- secret-safe proxy credentials;
- DNS configuration scaffold;
- local `.local` behavior preserved;
- TLS policy;
- explicit browser limitations;
- per-step diagnostics.

**Pass when:** supported desktop networking paths are tested; browser-only builds do not pretend to apply unsupported proxy/DNS overrides; `timekeeper.local` retains its local-resolution/fallback behavior.

---

## Gate 2 — Read-only shell and Project hierarchy

Use the existing health/project/execution-tree contracts to render Overview, Projects, and the nested Category/Task/Subtask/Sprint hierarchy.

Keep selection, filtering, and navigation as UI state; refresh from Core after navigation.

This gate is implemented in the current source, but future changes must retain its tests and read-only boundary.

**Pass when:** frontend tests, Svelte/TypeScript diagnostics, production build, desktop embed tests, and a browser smoke verify loading, empty/error/retry states, hierarchy ownership/depth, and workspace restoration.

Report browser-fixture evidence separately from an installed Wails runtime test.

---

## Gate 2A — Identity, authentication, and authorization foundation

Implement the Core/API and UI scaffolding required so later writable workspaces are not built around an anonymous-superuser assumption.

Requirements:

- authentication required toggle;
- username/password authentication;
- initial Owner bootstrap;
- user model;
- group model;
- role/permission evaluation;
- session lifecycle;
- human/agent/service/API-key principals;
- permission-aware UI shell;
- Core-side enforcement;
- emergency recovery path according to security design.

**Pass when:** authentication-disabled local behavior is explicit; authentication-enabled sessions work; unauthorized API calls are rejected even if manually issued; disabling UI controls is not the only authorization check; initial Owner bootstrap cannot trivially lock out the instance.

---

## Gate 2B — Registration, approval, and social/OIDC providers

Requirements:

- registration toggle;
- admin-approval toggle;
- pending registration state;
- approve/reject workflow;
- Microsoft toggle;
- Google toggle;
- GitHub toggle;
- Apple toggle;
- generic OIDC scaffold;
- account linking;
- provider secrets kept server-side;
- provider authentication separated from TimeKeeper authorization.

**Pass when:** a verified external identity cannot obtain normal access without satisfying TimeKeeper registration/approval policy; pending users cannot enter protected workspaces; administrators can approve/reject using permissions rather than hardcoded role checks.

---

## Gate 2B.1 — Directory framework, SCIM, and provider boundary

Implement the vendor-neutral directory foundation before adding enterprise-specific providers.

Requirements:

- external identity record/linking model;
- directory-source/provider contract;
- user/group/membership provisioning boundary;
- explicit attribute mapping;
- explicit external-group → TimeKeeper group/role mapping;
- provisioning/deprovisioning policies;
- sync checkpoint/history/status;
- audit attribution;
- generic SCIM 2.0 provisioning endpoint/contract;
- password data explicitly excluded from directory synchronization;
- provider-specific secrets isolated server-side;
- Microsoft/LDAP/other vendor connectors kept outside mandatory Core behavior.

Initial provider modules may include:

```text
Microsoft Enterprise
  ├── Entra / Graph directory sync
  ├── Teams
  └── Outlook integrations

Active Directory / LDAP
  └── LDAP / LDAPS
```

The Microsoft provider may use incremental/delta-style synchronization where supported by its external contract, but the TimeKeeper directory framework must not depend on Microsoft-specific semantics.

**Pass when:** Core can represent an external directory source without vendor-specific assumptions; SCIM can provision/disable users according to policy; removal/disablement preserves historical attribution; an external group cannot grant a TimeKeeper role without an explicit mapping; provider failure does not disable local users or ordinary TimeKeeper operation.

---

## Gate 2C — Administration workspace

Build the first-class Administration shell:

```text
Users
Groups
Roles & Permissions
Registrations
Authentication
Directory & Identity
Sessions
API Keys
Audit Log
Network
Logging & Diagnostics
Backup & Recovery
Store & Extensions
Integrations
Storage
System
Security
```

Panels may remain unavailable until their backing contract is implemented, but unavailable must be explicit.

**Pass when:** navigation is permission-driven; a non-admin role with a delegated permission can access only the appropriate administration surface; UI cannot bypass Core authorization.

---

## Gate 3 — Task and Sprint workspace

Build task detail from the Project execution tree and existing task/subtask/Sprint read contracts.

Show only fields returned by Core.

Add explicit lifecycle actions through existing validated routes; after each successful command, refresh from Core.

Do not introduce frontend status rules, direct database writes, or inferred data.

Add assignment UI only when the Core assignment contract exists.

**Pass when:** route/payload contracts, legal/illegal transition behavior, task/Sprint ownership, assignment authorization, refresh-after-write, and browser behavior are tested.

Existing API/Store validation remains authoritative.

---

## Gate 3A — TimeKeeper Inbox, assignments, and actionable messages

Requirements:

- persisted Core message model;
- Inbox UI;
- read/unread state;
- assignment messages;
- user/group recipients where supported;
- actionable-message framework;
- message-state transitions;
- security/approval/system categories;
- message links to Projects/Tasks/Subtasks/Sprints;
- audit attribution.

Initial built-in workflows should include at least:

- work assignment;
- registration approval;
- project invite;
- collaboration/access request;
- password/security reminder;
- backup failure;
- Store installation request.

**Pass when:** message state survives restart; delivery-provider failure does not lose the message; message actions are Core-authorized; assignments can create reliable Inbox items without frontend-only bookkeeping.

---

## Gate 3B — ntfy and email delivery

Requirements:

- optional ntfy backend;
- secure ntfy configuration;
- per-user delivery preferences;
- in-app message always retained;
- email opt-in;
- mandatory policy support for selected security messages;
- retry/failure reporting;
- no secret exposure in Svelte.

**Pass when:** a persisted TK message can be delivered by ntfy without ntfy becoming authoritative; email/push can be disabled per allowed policy; delivery failure leaves the Inbox message intact.

---

## Gate 3C — Native Calendar

Requirements:

- native Calendar workspace;
- timezone-safe timestamps;
- derived project/task/subtask/Sprint dates;
- explicit events;
- reminders;
- project/user scopes;
- permission checks;
- links back to TimeKeeper entities.

**Pass when:** derived events do not duplicate/change authoritative task dates independently; explicit meetings/reminders persist; timezone rendering is deterministic; calendar remains usable without any external calendar plugin.

---

## Gate 3D — Meeting/calendar integration contract

Define the provider boundary for external meeting/calendar systems.

Initial target provider may be Microsoft Teams, but the Core model remains provider-neutral.

**Pass when:** a TimeKeeper meeting survives provider unavailability/removal; provider references are supplemental; meeting invitations use the TK messaging system; provider secrets/permissions are isolated behind the integration boundary.

---

## Gate 4 — Activity, ToTb, Pulse, and metrics

Map each panel to existing persisted endpoints and document response freshness.

Reuse project-scoped ToTb and persisted events.

Keep human notes separate from system events, and keep reported values distinct from derived metrics.

Only add a Core read projection when the source already has the information but no usable contract exposes it.

**Pass when:** empty, unavailable, and configured states are distinct; every displayed value has a named API field and provenance; SSE/polling has bounded behavior; browser tests prove safe rendering and error states.

---

## Gate 4A — Logging and diagnostics administration

Requirements:

- default/custom log path;
- safe fallback if custom path is unavailable;
- per-component log levels;
- rotation/retention;
- separate audit policy;
- Open Log Folder;
- sanitized diagnostic bundle.

**Pass when:** invalid/unavailable log configuration cannot prevent Core startup; secrets do not appear in diagnostic bundles; audit retention is not accidentally governed by ordinary debug-log cleanup.

---

## Gate 4B — SQLite shadow recovery

Requirements:

- configurable shadow location;
- consistent SQLite snapshot API/mechanism;
- A/B shadow rotation;
- integrity verification;
- manual snapshot;
- recovery candidate validation;
- permission-protected restore.

**Pass when:** a shadow snapshot can be made while the Core is operating without raw-file corruption; an interrupted shadow update leaves at least one known-good candidate; restore never overwrites the primary before the candidate is verified.

---

## Gate 4C — Remote MySQL/MariaDB disaster-recovery backup

Requirements:

- optional remote SQL backup;
- MySQL/MariaDB configuration;
- TLS;
- Test Connection;
- Backup Now;
- exact radio intervals: 30m, 1h, 4h, 8h, 12h, 24h;
- optional missed-backup run on startup;
- recovery/history semantics;
- verification;
- failure notification through TK messaging;
- permission-protected restore.

**Pass when:** remote DB outage does not stop TimeKeeper; remote storage is never treated as a second live writer; backups can be verified and restored deterministically; failure produces an Inbox event; configured interval is honored.

---

## Gate 5 — Store and Extensions workspace

Treat Store as an optional distribution/entitlement client, separate from local project tracking and separate from the private Store server.

The Store UI may surface, through a narrow Go-side adapter backed by the existing `internal/storeclient` contract:

- Store connection/configuration state, including an honest not-configured/offline state;
- account and registered-device status, with secrets redacted;
- catalogue and entitlement results returned by the Store;
- explicit package verification and staging status, including product/version/hash and requested permissions.

Security and ownership rules:

- Svelte must not call Store endpoints directly or receive access/refresh tokens, signing keys, or package bytes as UI state.
- Reuse the existing Go Store client and its credential handling.
- Store-owned decisions—accounts, entitlements, product publication, signing, tickets, and server-side authorization—remain in the private `TimeKeeper-Store` service.
- The checked-in Store URL is a reserved example, not a production service address.
- “Install” must not silently mean “execute”.
- Core tracking and all non-Store workflows continue to work without an account, network, or Store deployment.

Add permission-governed Store/module management:

- users with `store.read` may browse;
- users with `store.request_install` may request installation;
- users with sufficient install/plugin permissions may approve/install/update/enable according to policy;
- security override remains highly restricted.

**Pass when:** Store protocol behavior is tested with local HTTP fixtures, auth state is kept out of the WebView, configured/unconfigured/denied states render honestly, package staging cannot activate a plugin without the separately gated mechanism, and permissions control installation actions.

---

## Gate 6 — Optional workspaces and native capabilities

Add Shadowbox, Memory Galaxy, plugin/settings, native dialogs, notifications, tray/background behavior, and other optional workspaces only through their owning Go/API/provider boundaries.

Memory Galaxy remains a cross-project memory subsystem; it is not a Store catalogue.

Keep failures isolated so an unavailable optional provider does not take down Core or the ordinary Project workspace.

**Pass when:** each optional panel has a real backing contract, a safe unavailable state, package/platform tests where relevant, and a browser/desktop smoke for its actual user path.

---

## Gate 7 — Packaging, parity, and legacy retirement

Build reproducible Wails artifacts for each supported platform.

Verify:

- embedded assets;
- local Core coexistence;
- remote connection mode;
- connection-profile persistence;
- authentication flows;
- permission-aware navigation;
- background operation;
- recovery/backup behavior where platform-specific;
- startup;
- memory;
- idle CPU;
- interaction latency;
- event freshness.

Treat performance numbers as hypotheses until measured; define cold/warm startup and foreground/background states before setting release budgets.

Retire the legacy UI only after:

- every required capability in the feature matrix is parity-verified;
- CLI/headless behavior remains intact;
- local mode is proven;
- required remote-mode behavior is proven;
- a rollback artifact and procedure exist;
- supported platforms are tested;
- optional Store/extension capabilities are explicitly classified as required, optional, or excluded from parity.

---

# 25. Settings design requirements

## 25.1 Personal Settings

```text
General
Appearance
Notifications
Personal Calendar preferences
Connection profile selection
Personal session/device view
```

## 25.2 Instance Administration

```text
Authentication
Registration
Users
Groups
Roles
Directory & Identity
API Keys
Network
Logging
Backup
Store
Plugins
System
Security
```

Do not merge personal preferences and instance governance into one undifferentiated settings form.

## 25.3 Save behavior

For every setting:

- identify the authoritative owner;
- validate server-side where applicable;
- return the saved authoritative representation;
- show restart/reconnect requirements explicitly;
- do not claim a change succeeded until Core confirms it;
- preserve previous working configuration on failed validation where practical.

---

# 26. Security rules

1. Raw passwords, OAuth secrets, API keys, proxy credentials, signing keys, ntfy credentials, and session tokens must not be persisted in Svelte/browser local storage.

2. Authorization is enforced by Core.

3. Remote unauthenticated mode may exist only as an explicit configuration and should display a strong warning. Remote/shared deployments should default toward authentication enabled.

4. Account registration and account authorization are separate.

5. External identity-provider authentication and TimeKeeper permission assignment are separate.

6. External directory provisioning and TimeKeeper authorization are separate. Directory connectors may provision identities, users, groups, and memberships but may not bypass explicit TimeKeeper role/permission policy.

7. Store installation and plugin activation are separate.

8. Backup configuration and restore are privileged operations.

9. Audit events must identify human/agent/service/API-key principals.

10. Diagnostics and support bundles must redact secrets.

11. Optional provider failure must degrade the provider capability rather than taking down normal Core operation.

---

# 27. Explicit non-goals

- Reorganizing `internal/store`, `internal/api`, `internal/cli`, or models into a new domain-package tree solely to match a proposed diagram.
- Creating a central application service, command layer, or event bus before a concrete tested need exists.
- Replacing SQLite as the authoritative TimeKeeper database.
- Making MySQL/MariaDB a live second writer.
- Implementing automatic bidirectional SQLite ⇄ MySQL synchronization.
- Implementing multi-master remote TimeKeeper databases as part of this UI migration.
- Changing stable IDs for presentation-only work.
- Making Wails, Svelte, the Store, ntfy, social-login providers, or any optional provider a prerequisite for CLI/headless Core operation.
- Direct SQLite or Store HTTP access from Svelte.
- Treating ntfy as the authoritative message store.
- Treating Microsoft Teams/Google/Zoom as the authoritative calendar.
- Hardcoding Microsoft Entra, Microsoft Graph, classic Active Directory, LDAP, Google Workspace, Okta, or another vendor directory into TimeKeeper's mandatory Core identity implementation.
- Synchronizing external-directory passwords into TimeKeeper.
- Letting an external directory group implicitly grant Owner/Admin privileges without an explicit TimeKeeper mapping.
- Automatically executing a Store-staged package.
- Shipping purchase/CRM/developer-publishing flows as part of the TimeKeeper desktop migration.
- Rewriting the Store backend merely because the Store portal may later use another frontend framework.
- Removing the legacy UI as part of a Wails shell milestone.
- Claiming browser proxy/DNS controls work where the browser cannot apply them.
- Silently falling back from a configured remote Core to a local Core.

---

# 28. Verification record for every gate

Record the exact source commit/tree state and separate these evidence levels:

1. focused unit/API/Store/auth/directory/SCIM/backup tests;
2. frontend test, type-check, and production build;
3. Go desktop/embed tests;
4. browser smoke with fixtures;
5. supported-platform Wails runtime/package smoke;
6. local Core integration smoke;
7. remote Core integration smoke where remote mode is in scope;
8. external-provider smoke only when an approved/configured provider exists;
9. Core or Store deployment verification, if that deployment is in scope.

A passing earlier level does not imply a later one.

Never report:

- an uninstalled desktop build as installed;
- an unconfigured Store as live;
- a fixture ntfy server as production delivery;
- a mocked social-login flow as a configured provider;
- a fixture SCIM/LDAP/Graph connector as a verified live enterprise-directory deployment;
- a successful directory connection test as proof that provisioning/deprovisioning policy is correct;
- a successful MySQL connection test as a verified restore;
- a generated shadow file as a healthy recovery point until integrity verification passes.

---

# 29. Feature matrix additions

The migration feature matrix should include at least:

| Capability | Core/API | Desktop UI | Browser UI | Headless/CLI | Permission | External dependency |
|---|---|---|---|---|---|---|
| Local Core connection | required | required | n/a/limited | required | n/a | none |
| Remote Core connection | required | required | required | required | profile policy | network |
| Primary/secondary endpoint | required | required | required | optional | settings | network |
| API-key auth | required | required | applicable | required | apikey policy | none |
| Username/password | required | required | required | optional | auth | none |
| OIDC providers | native generic OIDC | required | required | n/a | identity.providers.manage | provider |
| SCIM 2.0 provisioning | native protocol | admin status/config | admin status/config | optional | scim.manage | directory client |
| Directory provider framework | native | required admin UI | admin UI | optional | directory.* | none |
| Microsoft Entra/Graph directory | plugin/adapter | when installed | when installed | optional | directory.* | Microsoft |
| Active Directory / LDAP | plugin/adapter | when installed | when installed | optional | directory.* | AD/LDAP |
| Registration approval | required | required | required | optional | registrations.manage | none |
| Users/groups/roles | required | required | required | optional | granular | none |
| Inbox/messages | required | required | required | readable/actionable | granular | none |
| ntfy delivery | adapter | settings | settings | n/a | notifications/admin | ntfy |
| Email notifications | adapter | settings | settings | n/a | notifications/admin | ntfy/email |
| Calendar | required | required | required | optional | calendar.* | none |
| Teams meeting provider | adapter | required when configured | required when configured | optional | integration permissions | Microsoft |
| Proxy/DNS | native/Core | required | read-only/limited | optional | network.manage | environment |
| Log configuration | required | required | admin UI | optional | logging.manage | filesystem |
| SQLite shadow | required | required | admin status | optional | backup.manage | filesystem |
| Remote SQL backup | required | required | admin status | optional | backup.manage | MySQL/MariaDB |
| Store browse | existing client | required | optional | existing | store.read | Store |
| Module install request | required | required | optional | optional | store.request_install | Store |
| Module install/activate | gated | permission UI | optional | gated | granular | Store/launcher |

---

# 30. Suggested implementation sequence

Do not attempt all new capabilities simultaneously.

Recommended sequence:

```text
01  Refresh source/current-system audit
02  Maintain feature matrix
03  Stabilize Wails/Core lifecycle
04  Add connection-profile abstraction
05  Add remote endpoint + capability discovery
06  Add network/TLS diagnostic scaffold
07  Preserve/read-only Project hierarchy

08  Add authentication toggle + initial Owner bootstrap
09  Add users/groups/roles/permissions
10  Add sessions and API-key principals
11  Add registration + admin approval
12  Add generic OIDC provider scaffold
13  Add native directory-source/provider framework
14  Add generic SCIM 2.0 provisioning
15  Add Directory & Identity administration UI
16  Build remaining Administration shell
17  Add Microsoft Entra/Graph provider module when required
18  Add Active Directory/LDAP provider module when required

19  Complete Task/Subtask/Sprint workspace
20  Add assignment contracts
21  Add TimeKeeper Inbox/message persistence
22  Add actionable-message workflows
23  Add ntfy/email delivery

24  Add native Calendar
25  Add reminders
26  Add meeting-provider contract
27  Add Teams implementation through the Microsoft module when provider credentials/contracts are approved

28  Add logging/diagnostics settings
29  Add SQLite shadow A/B recovery
30  Add MySQL/MariaDB backup
31  Add restore verification/UI

32  Add permission-governed Store/module workflow
33  Add remaining Activity/ToTb/Metrics/Memory/Shadowbox workspaces
34  Complete tray/background/native capabilities
35  Run parity/performance/security/accessibility gates
36  Produce rollback artifacts
37  Retire legacy UI only after verified parity
```

Existing tracked task IDs remain authoritative where they already map to work. New features should receive their own tracked Project/Task/Subtask/Sprint items rather than being silently folded into unrelated UI tasks.

---

# 31. Definition of done

The extended migration is complete only when the required scope is explicitly classified and verified.

## Core boundary

- one authoritative Core;
- one authoritative SQLite owner;
- CLI/headless paths remain usable;
- Wails is a client/adapter, not a second Core;
- remote mode does not spawn/fall back to a local Core.

## Connection

- `timekeeper.local` is the preferred local endpoint;
- `127.0.0.1:1618` is the local fallback;
- remote primary/secondary endpoints work as configured;
- automatic failover verifies instance identity;
- capability/version incompatibility is explicit;
- API-key credentials are protected.

## Administration

- users/groups/roles/permissions are Core-enforced;
- registration approval works;
- authentication can be enabled/disabled according to policy;
- social/OIDC providers can be independently enabled/disabled;
- generic OIDC is not coupled to a specific vendor;
- SCIM provisioning is available through the native open-protocol boundary where in scope;
- external directory sources use explicit mappings and provisioning/deprovisioning policy;
- Microsoft Entra/Graph and AD/LDAP functionality remain optional modules rather than mandatory Core dependencies;
- external-user deprovisioning preserves historical TimeKeeper attribution;
- sessions/API keys are manageable;
- administration visibility/actions are permission-based.

## Messaging

- Inbox is durable;
- assignments generate appropriate messages;
- actionable workflows are Core-authorized;
- ntfy/email are optional delivery channels;
- provider failure does not lose messages.

## Calendar

- Project/Task/Subtask/Sprint dates can appear natively;
- explicit meetings/reminders work;
- timezone handling is correct;
- external meeting providers are optional adapters.

## Networking

- local and remote profiles support their required routing;
- proxy/DNS/TLS settings are bounded to platforms that can actually apply them;
- diagnostics expose the selected route and failures.

## Logging

- log paths are configurable;
- invalid paths safely fall back;
- audit logs are distinct;
- rotation/retention works;
- diagnostic bundles redact secrets.

## Backup

- live SQLite remains authoritative;
- A/B shadow recovery works and is verified;
- remote MySQL/MariaDB backup works;
- supported remote intervals are exactly 30m/1h/4h/8h/12h/24h;
- backup failure never prevents normal TimeKeeper operation;
- restore is permission-protected and verified.

## Store/extensions

- Store remains optional;
- browse/request/install permissions are distinct;
- staging does not imply execution;
- privileged users can manage modules without requiring unrelated administrator permissions.

## Migration quality

- no duplicated business rules in Svelte;
- no direct UI database writes;
- no silent provider success claims;
- every required feature has evidence at the appropriate test/runtime layer;
- legacy UI is retired only after required parity and rollback gates pass.

---

# 32. Architectural end state

```text
                                  TIMEKEEPER

                      ┌─────────────────────────┐
                      │       Go Core/API       │
                      └────────────┬────────────┘
                                   │
         ┌─────────────────────────┼────────────────────────────┐
         │                         │                            │
      SQLite                    Services                     Events
         │                         │                            │
   ┌─────┴─────┐      ┌───────────┼────────────────┐           │
   │           │      │           │                │           │
Primary      Shadow  Projects    Identity        Messages      │
DB           A / B   Tasks       RBAC            Calendar      │
   │                 Sprints     Sessions         Backup        │
   │                    │           │                │           │
   └────────────────────┴───────────┴────────────────┴───────────┘
                                   │
                         Versioned TimeKeeper API
                                   │
              ┌────────────────────┼─────────────────────┐
              │                    │                     │
          Wails/Svelte          Browser UI            CLI/Agents
              │                    │                     │
         Local/Remote           Remote/Core           Local/Remote
              │
     Connection Profile
     ├── endpoint(s)
     ├── auth
     ├── proxy/DNS/TLS
     └── capabilities

Native identity protocols:
  ├── Local users
  ├── Generic OIDC
  └── Generic SCIM 2.0

Optional integrations/modules:
  ├── Microsoft Enterprise
  │   ├── Entra / Graph
  │   ├── Teams
  │   └── Outlook
  ├── Active Directory / LDAP
  ├── TimeKeeper Store
  ├── ntfy/email
  ├── external calendars/meetings
  ├── Shadowbox
  ├── Memory Galaxy providers
  └── plugins

Recovery:
  Primary SQLite
       ↓
  Local SQLite Shadow A/B
       ↓
  Remote MySQL/MariaDB backup
```

The central rule remains:

> **Do not redesign TimeKeeper to accommodate Wails. Adapt Wails and the new UI to TimeKeeper's authoritative Core/API boundaries, while establishing the identity, administration, messaging, scheduling, networking, and recovery contracts now so the application does not require another structural UI rewrite later.**


---

# 33. Suggested solution plan — identified gaps

## 33.1 API rate limiting / abuse prevention

**Gap**: No rate limiting defined for the versioned API. Remote clients, CLI tools, or agents could overwhelm Core.

**Suggested solution**:
- Token-bucket per principal (user, agent, service, API client) with configurable burst and sustained rate.
- Separate buckets for read vs write operations (writes cost more).
- `429 Too Many Requests` with `Retry-After` header.
- Core-enforced at the HTTP middleware layer, not in Svelte.
- Configuration in Administration → System → Rate Limiting.

## 33.2 Data retention / event purging

**Gap**: Persisted project events grow unbounded. No retention or archival policy.

**Suggested solution**:
- Configurable retention window per event category (e.g., activity 90d, audit 1y, system 30d).
- Nightly pruning job inside Core (not a cron dependency).
- Archived events moved to a compressed `events_archive` table before deletion.
- Administration → System → Data Retention shows counts and next scheduled purge.
- Export-before-delete option for compliance.

## 33.3 Offline mode for Wails desktop

**Gap**: Plan says "optional services optional" but does not define offline behaviour when the Core is unreachable.

**Suggested solution**:
- Wails local cache (SQLite or embedded KV) for last-known server state.
- Read-only workspace browsing when offline; explicit offline indicator in UI.
- Write queue with conflict detection on reconnect (last-write-wins with manual merge prompt).
- Not a competing Store — purely a client-side cache with Core as the sync authority.
- Document which workspaces support offline read vs those that require live Core.

## 33.4 Concurrent edit conflict resolution

**Gap**: Two users (or an agent + user) editing the same task/sprint simultaneously has no defined conflict strategy.

**Suggested solution**:
- Optimistic locking via `updated_at` / version number on every mutable entity.
- API returns `409 Conflict` with current server state; UI shows diff and lets the user choose.
- Server never silently overwrites — the second writer must explicitly reconcile.
- Agent writes (FASE claims) already serialize through the sprint state machine; human-vs-human needs the version check.
- Document the rule: agents own lifecycle transitions; humans own content edits; conflicts surface to the human.

## 33.5 Webhook delivery guarantees

**Gap**: ntfy/email/webhook delivery has no defined retry, ordering, or dead-letter semantics.

**Suggested solution**:
- At-least-once delivery with exponential backoff (1m, 5m, 30m, 2h, 8h) — max 5 retries.
- Dead-letter table for permanently failed deliveries with manual redelivery action.
- Ordering: per-principal ordered delivery via sequence number; consumers must tolerate gaps.
- Idempotency key on every delivery so consumers can deduplicate.
- Delivery status visible in Messages → Administration → Delivery Log.

## 33.6 Calendar sync conflict handling

**Gap**: Native TimeKeeper calendar + external calendars (Outlook, Google) have no defined precedence or conflict rules.

**Suggested solution**:
- Native calendar is authoritative for TimeKeeper-scoped events (tasks, sprints, deadlines).
- External calendars are read-only overlays unless explicitly imported.
- Two-way sync only for meeting-provider integrations (Teams) where the meeting is created from a TimeKeeper calendar entry.
- Conflict rule: native wins for start/duration changes; external wins for attendee-only updates.
- Sync direction and conflict policy configurable per calendar link.

## 33.7 File version retention limits

**Gap**: File History versions accumulate without bound; no retention or pruning policy.

**Suggested solution**:
- Per-project configurable retention (e.g., keep last N versions, or versions from last M days).
- Automatic pruning of superseded versions when a new capture occurs.
- Exclusion patterns (e.g., `node_modules`, `*.lock`, build artifacts) to prevent noise.
- Administration → Storage shows per-project version counts and estimated disk usage.
- Pruning is a maintenance action — never deletes the latest version.

---

## 33.8 Implementation priority

| Gap | Priority | Depends on |
|---|---|---|
| 33.4 Concurrent edit conflicts | **High** | Core API version fields |
| 33.1 API rate limiting | **High** | Auth/RBAC (§7) |
| 33.2 Event retention | Medium | Core maintenance jobs |
| 33.5 Webhook delivery | Medium | Messages/Inbox workspace |
| 33.7 File version retention | Medium | Files & History workspace |
| 33.3 Offline mode | Medium | Connection profiles (§6) |
| 33.6 Calendar sync conflicts | Low | Calendar workspace + providers |

Each gap should be tracked as its own Task under the TimeKeeper Project tree with the same parity/evidence gates as the main migration plan.
