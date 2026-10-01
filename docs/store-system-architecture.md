# TimeKeeper Store system architecture

Status: design baseline for implementation. Sections marked **Implemented** describe
code that exists and is tested. Sections marked **Designed** are contracts and
boundaries that must be agreed before activation work proceeds.

This document is the system-level companion to [store.md](store.md). It covers
more than package download: identity, CRM, API, authentication, package
supply-chain, queues, logging, audit, security, connectors, lifecycle, and
recovery.

## 1. Non-negotiable invariants

1. Core remains useful without a Store account or network access.
2. Core is a stable host. A plugin cannot edit Core, import Core private code,
   mutate the Core database, or add host semantics at runtime.
3. A Store signature proves package provenance; it does not grant execution,
   permissions, filesystem access, network access, or database access.
4. Core and a plugin are separate processes. Communication uses only declared,
   versioned connectors.
5. The Store is the authority for accounts, entitlements, product publication,
   and download tickets. The client requests; the server decides.
6. Every mutating operation is authenticated, authorized, bounded, auditable,
   idempotent where retried, and safe to replay or refuse.
7. Every queue has an owner, a bounded payload, retry policy, dead-letter path,
   and operator-visible state.
8. Logs never contain passwords, tokens, private keys, package contents, or
   unbounded prompt/transcript data.
9. Implemented behavior and designed behavior are labelled separately in docs,
   tests, and release notes.

## 2. System boundary

```text
+----------------------+       HTTPS / v1 API       +----------------------+
| TimeKeeper Core      | --------------------------> | Store gateway        |
|                      |                             | auth, rate limits,   |
| local projects,      |                             | route allowlist       |
| ToTb, permissions,   |                             +----------+-----------+
| staging, supervisor  |                                        |
+----------+-----------+                                  private tunnel
           |                                              / authenticated
           | control stdio + capability loopback HTTP     v
           v                                      +----------------------+
+----------------------+                           | Store origin         |
| Plugin box           |                           | accounts, CRM,       |
| one external plugin |                           | entitlements, files, |
| own process/memory   |                           | tickets, audit       |
+----------------------+                           +----------+-----------+
                                                                    |
                                                                    v
                                                            MariaDB / SQLite
```

The plugin box never talks directly to the Store database. A plugin may use an
external service only when its granted capability and product policy explicitly
permit it. The default is no network access.

## 2.1 Plugin egress containment — **Designed, activation blocker**

Raw outbound connectivity is forbidden. `network.egress` is not a blanket
internet permission; it is a Core-approved policy attached to a supervised
plugin process. The policy includes exact destination hostnames, ports,
methods, redirect targets, request/response byte caps, requests-per-minute,
concurrent-request limits, and an expiry/revocation handle.

The Core-owned egress broker is the mandatory network path. The plugin sees
only connector operations; Core resolves DNS, establishes TLS, applies the
allowlist and quotas, and performs the outbound request. OS-level per-process
firewall/network isolation is defense in depth only, never the authority. A
platform that cannot establish Core mediation must refuse plugin activation.

The connector records bounded metadata—plugin/product id, destination, method,
status, bytes, quota result, correlation id, and denial reason—without recording
request/response bodies by default. It never passes Store/Core tokens to a
plugin. Quota exhaustion or bypass attempts revoke the connector and stop the
plugin, producing an audit event.

## 3. Component ownership

| Component | Owner | Responsibility |
|---|---|---|
| Core host | TimeKeeper Core | stable contracts, launcher policy, grants, supervision, local state |
| Store gateway | Store | public API boundary, auth enforcement, rate limiting, route allowlist |
| Store origin | Store | business decisions, database access, package metadata, ticket issue |
| Package signer | Store operator | isolated Ed25519 private key and publication ceremony |
| Plugin box | plugin developer | implementation, bridge behavior, health, bounded work |
| Core connector adapters | Core release | fixed request/response schemas and permission checks |
| CRM/support surface | Store operator | account/developer/device/customer lifecycle and support evidence |
| Audit/observability | both systems | security events, technical logs, correlation and health |

No component gets authority merely because it can send a request. Authority is
checked at the boundary that owns it.

## 4. API design

### 4.1 Versioning — **Implemented**

The Store exposes `/v1`. Clients send the Store API version and the Store
reports accepted versions from `/v1/meta/versions`. Breaking changes require a
new version. Additive response fields are tolerated; unknown request fields are
rejected.

### 4.2 Public routes — **Implemented**

- `GET /health`
- `GET /v1/meta/public-key`
- `GET /v1/meta/versions`
- `POST /v1/auth/register`
- `POST /v1/auth/login`
- `POST /v1/auth/refresh`
- `POST /v1/auth/logout`

### 4.3 Authenticated routes — **Implemented**

- `GET /v1/account`
- `GET/POST /v1/account/devices`
- `GET /v1/account/entitlements`
- `GET /v1/store/extensions`
- `GET /v1/store/extensions/{id}`
- `POST /v1/store/claim`
- `POST /v1/store/install`
- `POST /v1/download/request`
- `GET /v1/download/{ticket}`

### 4.4 Designed future route groups

These are reserved seams, not active promises:

- `/v1/developer/*` — developer identity, products, versions, release review;
- `/v1/crm/*` — support-safe account/device/license timeline and case metadata;
- `/v1/billing/*` — payment provider webhooks and reconciliation;
- `/v1/notifications/*` — preference and delivery status;
- `/v1/admin/*` — operator-only actions, never exposed through the client.

Each group needs its own role, payload schema, audit events, rate limits, and
idempotency rules before implementation.

## 5. Authentication and authorization

### 5.1 Identity model — **Implemented**

- `users` hold public account identity, email, password hash, display name, and
  status (`active`, `suspended`, `deleted`).
- `devices` hold revocable installation identities (`TK-DEVICE-...`), optional
  public keys, labels, and last-seen data.
- `sessions` hold only a refresh-token hash and bounded session metadata.
- Access tokens are short-lived; refresh tokens rotate and old values cannot be
  replayed.
- Passwords and signing private keys never leave their owning service.

### 5.2 Boundary checks — **Implemented**

Every protected request passes through:

1. route allowlist;
2. content-type and body-size limit;
3. JSON schema and unknown-field rejection;
4. freshness timestamp and nonce checks for mutations;
5. bearer/session authentication;
6. device revocation check;
7. resource ownership/entitlement decision;
8. audit event attempt.

The client may verify package signatures locally. It cannot self-assert
ownership, entitlement, price, or device authority.

### 5.3 Roles and CRM authorization — **Designed**

Separate these roles; do not reuse a customer token as an operator credential:

- customer: own account, devices, entitlements, downloads;
- developer: own product metadata and submitted releases;
- support: read a redacted customer timeline and create case notes;
- operator: publish/delist/revoke and manage incidents;
- billing worker: reconcile provider events only;
- system: bounded internal jobs.

Every role gets an explicit route/resource matrix and an audit event. Support
must not see passwords, token values, private keys, raw package bytes, or private
project/ToTb content.

## 6. CRM and customer lifecycle

### 6.1 Existing data foundation — **Implemented**

The Store schema already has the durable foundation:

```text
users -> devices -> sessions
users -> entitlements -> products -> product_versions -> product_files
users -> licenses
users -> transactions / subscriptions
users -> downloads
users/developers/system -> audit_events
```

`developers`, `developer_products`, product status, entitlement sources,
license seats, transactions, and subscriptions are present as future-safe
scaffolding. Payments are not active yet.

### 6.2 CRM surface — **Designed**

CRM is a support and lifecycle view, not a second authority. It should project
from authoritative Store records into:

- account profile and status timeline;
- device registration/revocation timeline;
- entitlement/license history;
- product/version/download history;
- consent and notification preferences;
- support case, assignment, severity, status, and redacted notes;
- incident and communication history.

A CRM case must reference stable public IDs and audit event IDs, not copy secrets
or package contents. Customer deletion/export must define retention and legal
holds before the CRM tables are implemented.

## 7. Package supply chain

### 7.1 Build and signing — **Implemented**

The private Store builder creates deterministic `.tkp` archives, generates
`checksums.json`, hashes the final archive, and signs:

```text
timekeeper-package/v1:<archive sha256 hex>
```

The private Ed25519 key stays in the Store operator environment. The client
receives only the public verification key.

### 7.2 Publication state — **Implemented/Designed split**

Implemented product states: `draft`, `published`, `hidden`, `delisted`.
Implemented version channels include `stable` and `beta`.

Designed release gates:

```text
draft -> automated checks -> security review -> staged -> published
published -> hidden/delisted/revoked
```

A release gate must verify manifest identity, version uniqueness, package size,
archive traversal safety, signature, declared permissions, bridge compatibility,
and a clean install/handshake test. It must also pass the security scanning
pipeline in [plugin-security-scanning.md](plugin-security-scanning.md): static
destructive-code checks, dependency/SBOM review, and hermetic dynamic behavior
checks. Delisting stops new catalogue/download access; it must not silently
destroy already staged local data.

### 7.3 Client installation — **Implemented, staging only**

```text
install request -> entitlement -> single-use ticket -> stream
-> checksum -> signature -> bounded extraction -> manifest match -> staged
```

Activation remains gated behind the trusted launcher and connector adapters.

## 8. Queues and asynchronous work

### 8.1 Synchronous request path — **Implemented**

Authentication, entitlement checks, ticket creation, signature metadata, and
bounded response validation remain synchronous because the caller needs a clear
answer. They must not wait on plugin execution, email, payment providers, or
unbounded package scanning.

### 8.2 Durable queue topology — **Designed**

The Store should use explicit queues with bounded JSON envelopes:

| Queue | Producer | Consumer | Retry/DLQ |
|---|---|---|---|
| `package.publish` | developer/operator release | package verifier/signer | 3 retries, then review DLQ |
| `package.scan` | publication | archive/security scanner | 3 retries, quarantine on failure |
| `download.reconcile` | ticket delivery | usage/audit reconciler | idempotent replay |
| `billing.events` | payment provider | entitlement reconciler | signature check, DLQ |
| `crm.notifications` | account/support events | notification worker | channel-specific retry |
| `audit.export` | operator/compliance request | redacted exporter | no silent drop |
| `plugin.lifecycle` | Core launcher | supervisor/adapter | local bounded retry |

Every message contains `message_id`, `schema_version`, `correlation_id`,
`causation_id`, `created_at`, `attempt`, `not_before`, `expires_at`, and a
bounded payload. Secrets and package bytes do not travel through queues.

### 8.3 Queue rules — **Designed**

- Claim with a lease; acknowledge only after durable side effects succeed.
- Make consumers idempotent using `message_id` plus domain idempotency keys.
- Use exponential backoff with a maximum delay and absolute expiry.
- Move poison messages to a dead-letter queue with a reason and safe summary.
- Preserve ordering only where the domain requires it; do not serialize the
  entire Store behind one global queue.
- Expose queue depth, oldest age, retry count, and DLQ count as metrics.
- Never use a queue as a substitute for authentication or authorization.

## 9. Core/plugin connectors

### 9.1 Control connector — **Contract implemented**

Newline-delimited JSON over child-process stdin/stdout. Methods:

- `hello`, `hello_ok`;
- `health`, `health_ok`;
- `shutdown`;
- `error`.

Stdout is protocol-only. Logs use stderr or a configured sink.

### 9.2 Capability connector — **Contract designed, adapters gated**

JSON over the explicit numeric-loopback HTTP origin assigned by Core. Each
capability has a fixed versioned route, request/response schema, timeout, and
permission mapping. The plugin cannot select any of these values.

The handshake allowlist is frozen. Core refuses undeclared capabilities before a
request reaches the plugin. A dead, hung, incompatible, or unauthorized plugin
becomes `unavailable`; Core continues normally.

### 9.3 Store/API connector

Core talks to the Store through the versioned HTTPS API/gateway only. A plugin
must not reuse Core's Store session or call private Store endpoints. If a plugin
needs Store data, Core must expose an approved capability that returns the
minimum redacted data.

## 10. Logging, audit, and observability

### 10.1 Two logging modes — **Designed baseline**

Normal mode records bounded operational facts:

- timestamp, level, component, event, correlation ID;
- route/operation shape, status, duration;
- product/version public IDs;
- queue name/message ID and outcome;
- plugin ID/capability and health state.

Debug mode additionally records command/step transitions, validation failure
class, retry calculations, connector frame metadata, and sanitized stack/error
context. It must never record secrets or full payloads by default. Debug mode is
explicit, bounded, and operator-controlled; it is not a permanent verbosity
switch.

### 10.2 Audit trail — **Implemented foundation**

`audit_events` is append-only and records actor type/id, event, target, safe
detail JSON, IP, and UTC time. Audit writes must be loud on failure. Security
critical operations must define whether an audit failure blocks the mutation or
creates a durable compensating alert; this policy must be made explicit before
payments/admin actions ship.

### 10.3 Correlation

Carry a non-secret `correlation_id` from API request through Store service,
queue message, Core install, launcher, bridge session, and plugin operation.
Never use access tokens or package contents as correlation IDs.

### 10.4 Required metrics and alerts

- auth failures, refresh replay, revoked-device attempts;
- entitlement denials and ticket replay/expiry;
- package checksum/signature failures;
- queue depth, age, retry, and DLQ counts;
- bridge handshake failures, capability refusals, crashes, and timeouts;
- install staging failures and disk-limit refusals;
- audit write failures;
- CRM notification failures and support backlog.

## 11. Threat model and controls

| Threat | Control |
|---|---|
| modified client asserts ownership | server-side entitlement authority |
| stolen refresh token | hash at rest, rotation, replay revocation |
| revoked device downloads | device check on every protected path |
| replayed mutation | timestamp + nonce + idempotency key |
| tampered package | archive hash + detached signature + checksums |
| archive traversal/symlink | bounded regular-file extraction and root checks |
| malicious plugin reaches Core DB | separate process, no DB handle, fixed connectors |
| plugin widens capability | frozen handshake allowlist |
| package chooses command/port/path | trusted Core launcher policy |
| queue poison/replay | bounded schema, leases, idempotent consumers, DLQ |
| log secret leakage | redaction policy and secret-scanning tests |
| Store origin bypass | production gateway and origin authentication |

Security exceptions require a recorded decision, owner, expiry, and compensating
control. “It is local” is not a control.

## 12. Lifecycle and failure semantics

```text
not installed
  -> entitlement available
  -> ticket issued
  -> verified and staged
  -> launcher eligible
  -> handshake accepted
  -> healthy
  -> capability calls
  -> draining
  -> stopped / unavailable
```

Failure behavior:

- Store unreachable: Core keeps local functionality; installation remains
  incomplete and retryable.
- Ticket expired/replayed: refuse; request a new ticket.
- Signature/checksum mismatch: delete the rejected archive and quarantine the
  install attempt; never execute.
- Extraction limit/path violation: refuse and clean partial staging.
- Bridge mismatch: mark unavailable; do not downgrade silently.
- Plugin crash/hang: supervisor contains it, records a safe event, and applies
  bounded restart policy.
- Capability timeout: fail the operation, preserve Core health, and avoid
  infinite retry loops.
- Queue poison: dead-letter with a safe reason and require operator action.
- Account suspension/revocation: stop new Store authority immediately; define
  separately whether already installed local plugins are disabled by policy.

## 13. Release gates

No Store runtime activation release is complete until all are true:

- API schemas and error codes are documented and contract-tested;
- auth/device/session/nonce/replay behavior is tested;
- CRM role/resource matrix and redaction policy are approved;
- package builder is reproducible and signing key custody is documented;
- queue envelopes, leases, retries, idempotency, and DLQs are specified;
- normal/debug logging and audit events are defined with redaction tests;
- package scanner stages, thresholds, SBOM evidence, quarantine, and rescan policy are implemented;
- bridge handshake and every refusal path are tested;
- connector schemas and permission mappings are versioned;
- clean install proves staging without execution;
- supervised launch proves actual work, health, timeout, shutdown, and crash
  containment;
- recovery and rollback are exercised from recorded evidence;
- docs map links resolve and implemented/designed status is current.

## 14. Current status

Implemented and tested today: Store v1 API/auth/entitlement foundations, signed
package delivery, client verification/extraction, bridge handshake/refusal
contract, staging-only install, and base audit/schema foundations.

Designed but not yet fully implemented: the generic trusted launcher,
Core-mediated egress broker and quotas, connector-specific capability adapters,
package scanner/quarantine pipeline, durable Store queues/DLQs, CRM case and
notification surfaces, complete debug logging contract, billing activation, and
full plugin work execution.

The design intentionally comes before activation. Otherwise we would be
building a plugin marketplace with the architectural equivalent of loose
cutlery in a tumble dryer.
