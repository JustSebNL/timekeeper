# .tkp package format

A `.tkp` is a gzip-compressed tar archive. Official Store packages are
signed; TimeKeeper verifies the signature before anything is unpacked.

```
extension.memory-galaxy-0.1.0.tkp
├── manifest.json     required — id, version, permissions, entrypoint
├── extension.js      the code the extension runtime loads
├── assets/           optional static assets
├── migrations/       optional SQLite migrations
├── package.json      optional runtime dependency declarations
├── package-lock.json required when package.json declares runtime dependencies
└── checksums.json    generated — sha256 of every other file
```

`checksums.json` is generated at build time and never hand written. It
does not include itself.

## manifest.json

```json
{
  "id": "extension.memory-galaxy",
  "name": "Memory Galaxy",
  "version": "0.1.0",
  "min_app_version": "1.5.0",
  "runtime": "timekeeper-extension",
  "entrypoint": "extension.js",
  "permissions": ["projects.read", "totb.read", "memory.read"]
}
```

`id` and `version` are required. Permissions are a request, not a
grant: TimeKeeper Core decides what the extension receives.

## Dependency evidence

`package.json` is optional. If it declares runtime dependencies, the archive
must also contain `package-lock.json`. The Store's dependency scan does not
resolve or download packages: it checks the checked-in lock inventory for a
version, HTTPS registry source, integrity checksum, and allowlisted license for
each dependency. It emits a deterministic `timekeeper-sbom/v1` inventory with
`required_by` relationships and stores its SHA-256 digest in the immutable scan
record. Missing lock entries, lock drift, untrusted registries, missing
licenses, and disallowed licenses fail publication before signing.

## Signing

The Store signs a canonical string, not the archive bytes:

```
timekeeper-package/v1:<sha256 hex of the archive>
```

ed25519, detached, base64. The signature travels **outside** the
archive — in the Store record and in the `X-TK-Signature` response
header — because an archive cannot sign itself.

The verification key is public and published at
`GET /v1/meta/public-key`:

```json
{
  "algorithm": "ed25519",
  "format": "spki-pem",
  "public_key": "-----BEGIN PUBLIC KEY-----…",
  "fingerprint": "924f0c7fdb1f702a633e6b062758c609"
}
```

The private key never leaves the Store. TimeKeeper Core ships only the
public half.

## Verification order

1. `POST /v1/store/install` → manifest, sha256, signature, ticket
2. stream the package with the ticket
3. hash the bytes, compare with the signed checksum
4. verify the ed25519 signature over the canonical string
5. only then extract

A mismatch, a missing signature or a foreign key leaves nothing behind
but the rejected archive.

Verification is necessary but not sufficient for activation. After extraction,
the package remains staged data until Core starts it through the supervised
two-box bridge described in `docs/two-box-architecture.md`. Core must never
`exec` an archive entry directly, import downloaded code, pass a database
handle, or create a connector from package-controlled input. The bridge
launcher chooses the executable, loopback port, capability allowlist, timeout,
filesystem grants, and shutdown policy from trusted Core configuration.

A `.tkp` is a delivery artifact, not a Core module. The complete developer
workflow—including how to implement the external bridge process—is in
[store-developer-guide.md](store-developer-guide.md).

## Extraction limits

| Limit        | Value   |
| ------------ | ------- |
| files        | 500     |
| per file     | 32 MB   |
| unpacked     | 64 MB   |

Symlinks, device nodes and any entry whose name escapes the
destination directory are refused. After extraction the client
re-checks every file against the `checksums.json` inside the archive.

## Download tickets

Packages have no permanent public URL. The client asks for a ticket,
the Store checks the entitlement, and the ticket expires in seconds
(`TK_DOWNLOAD_TTL`, default 120) and is single use. A ticket is bound
to the account that requested it.
