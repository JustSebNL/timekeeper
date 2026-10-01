# Store plugin security scanning

A package signature answers “who published these bytes?” It does not answer
“what do these bytes do?” Every Store plugin release therefore passes a scanner
pipeline before publication. Scanning lowers risk; it never replaces the Core
process boundary, egress broker, permission checks, or runtime supervision.

## Scan state machine

```text
submitted
  -> structurally_validated
  -> statically_scanned
  -> dependency_scanned
  -> dynamically_scanned
  -> risk_scored
  -> human_review (when required)
  -> approved / rejected / quarantined
  -> signed_and_published
```

A package hash is the immutable identity of a scan. Replacing any package byte,
manifest, dependency lockfile, or scanner policy requires a new scan. A cached
result may be reused only when the package hash, scanner versions, policy
version, and tool databases are all still valid.

## Scanner stages

### 1. Structural and manifest scan

Before code is executed or trusted:

- verify gzip/tar format and bounded compressed/unpacked sizes;
- reject absolute paths, traversal, symlinks, device nodes, duplicate entries,
  hidden executable payloads, and archive bombs;
- parse one strict manifest with no unknown security fields;
- validate product id, version, runtime, entrypoint metadata, and permissions;
- require the external bridge runtime for Store plugins;
- reject permissions not in the stable Core vocabulary;
- reject an egress policy embedded by the package—egress is Core-owned;
- ensure manifest identity matches the Store product/version record.

### 2. Static destructive-code scan

Use language-aware parsing where possible, plus conservative signatures and
entropy/obfuscation checks. Findings are evidence for policy, not proof of
intent. Scan source, bundled assets, generated code, native binaries, scripts,
lockfiles, and compressed nested archives.

Critical findings requiring rejection or security review include:

- raw socket, packet, DNS, proxy, or tunnel creation outside the Core egress
  connector;
- process spawning, shell invocation, command construction, or system-tool
  execution not covered by a pre-approved capability;
- recursive deletion, disk/device writes, partition/filesystem operations,
  destructive migrations, or writes outside the staged/plugin data root;
- credential, browser-cookie, SSH-key, environment-secret, token, or private-key
  harvesting;
- persistence, autorun, service/task creation, privilege escalation, debugger
  attachment, or security-tool disabling;
- arbitrary code download/evaluation, self-modifying code, hidden loaders, or
  obfuscation that prevents review;
- attempts to access Core databases, ToTb/private project files, process memory,
  or unrelated user home directories;
- data collection or upload loops, archive/encryption staging of large local
  trees, or covert channels.

A scanner must understand benign test fixtures and document suppressions. A
suppression names the finding, file/hash, reason, reviewer, expiry, and the
narrow capability that makes it safe. “False positive” alone is not a waiver.

### 3. Dependency, SBOM, and license scan

Produce a dependency inventory with package name, version, source, checksum,
license, and transitive relationship. Check:

- known vulnerabilities against the configured advisory database;
- abandoned or unmaintained critical dependencies;
- dependency confusion and unexpected registries;
- embedded native binaries and their provenance;
- incompatible or undisclosed licenses;
- lockfile/package checksum drift.

The SBOM digest is attached to the scan record and release audit event. A
runtime dependency downloaded after installation is not allowed unless the
release policy explicitly supports it through a Core-mediated connector.

### 4. Hermetic dynamic scan

Run the candidate in a disposable sandbox with:

- no Core database, project files, credentials, Store tokens, or host secrets;
- a fake `timekeeper-bridge/v1` control tunnel;
- the Core egress broker in deny-all mode unless the test explicitly grants a
  test destination;
- canary files and synthetic records containing unique non-secret markers;
- read-only package files and a bounded writable data directory;
- CPU, memory, process, file, disk, duration, and output limits;
- syscall, process, file, DNS, socket, and connector event tracing;
- deterministic fake API responses and a kill switch.

Exercise startup, handshake, health, normal capability calls, malformed input,
timeout, shutdown, crash, quota exhaustion, and connector refusal. Any canary
read, unexpected process, raw socket, unauthorized file write, direct network
attempt, or policy bypass is a finding.

Dynamic scanning must test the negative path: the plugin should be unable to
send data when the Core egress broker denies it, even if the plugin attempts a
raw socket, DNS lookup, alternate proxy, IP literal, redirect, or subprocess.

## Risk policy

Each finding has:

- severity: critical, high, medium, low, or informational;
- confidence: confirmed, probable, or heuristic;
- evidence: file/offset, trace event, or dependency record;
- policy rule and scanner version;
- disposition: reject, fix-and-rescan, review, or accepted with expiry.

Default release decisions:

| Result | Decision |
|---|---|
| any confirmed critical finding | reject and quarantine |
| confirmed high finding | reject or mandatory security review |
| dynamic policy bypass | reject; no waiver by plugin developer |
| raw egress attempt | reject unless the exact Core connector test proves mediation |
| medium findings | manual review with explicit disposition |
| low/informational | publish only with recorded evidence and owner |
| scanner unavailable or incomplete | do not publish |

The thresholds are policy configuration owned by the Store/Core security
maintainers, not by package metadata.

## Scan record

Persist a redacted scan record keyed by package hash:

```json
{
  "package_sha256": "...",
  "product_id": "vendor.example",
  "version": "1.2.3",
  "scanner_version": "scanner/v1",
  "policy_version": "plugin-policy/v1",
  "sbom_sha256": "...",
  "static_result": "pass",
  "dependency_result": "pass",
  "dynamic_result": "pass",
  "findings": [],
  "decision": "approved",
  "reviewer": "security-review-id",
  "scanned_at": "2026-01-01T00:00:00Z"
}
```

Do not store source secrets, full request/response bodies, credentials, or
unbounded trace output in the Store audit record. Large evidence belongs in
quarantined operator storage referenced by a safe evidence id.

## Operator workflow

1. Receive a package submission into quarantine storage.
2. Compute its hash before unpacking.
3. Run all scanner stages with a correlation id.
4. Review findings and generated SBOM.
5. Approve, reject, or request changes; record the decision and expiry.
6. Sign and publish only the exact approved hash.
7. Re-scan on every version, manifest, dependency, scanner-policy, or relevant
   advisory change.
8. Delist or revoke when a new advisory invalidates an installed release.

The signer must consume the approved artifact by hash, not “whatever file is in
the package directory.” A scan pass for one archive must never authorize a
substituted archive.

## Developer release checklist

Before submission:

- remove debug credentials, local paths, test keys, and secrets;
- provide source and lockfiles needed for review;
- document every permission and why it is required;
- document every Core connector and expected data volume;
- prove network access is impossible without the Core egress broker;
- test destructive-operation refusals and clean shutdown;
- keep stdout protocol-only and send bounded logs to stderr;
- provide a reproducible build command and exact artifact hash;
- fix findings rather than requesting broad suppressions.

## What scanning cannot prove

Static and dynamic scans can miss conditional, delayed, encrypted, or novel
behavior. Therefore the non-bypassable controls remain:

- separate process and staged filesystem;
- Core-mediated egress only;
- frozen capability allowlist;
- explicit permission and data minimization checks;
- quotas, timeouts, supervision, and kill switch;
- redacted normal/debug logs and append-only audit events.
