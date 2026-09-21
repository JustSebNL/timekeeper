# TimeKeeper Store authentication protocol

Local TimeKeeper never requires an account. Authentication exists only
for Store functionality: browsing extensions, claiming, installing,
account and device management.

```
Local TimeKeeper
      ├── core functionality ── no account
      └── open Store ── login ── Store access
```

## Authority model

TimeKeeper is open source, so any client can be modified. The Store
therefore treats every request as untrusted:

- no request field may assert ownership;
- unknown fields are rejected before business logic runs;
- entitlements are resolved only from the Store database;
- the client may verify package signatures locally, nothing else.

> The client requests. The server decides.

## Tokens

| Token         | Lifetime | Storage                                  |
| ------------- | -------- | ---------------------------------------- |
| access token  | 15 min   | memory + local session file              |
| refresh token | 30 days  | local session file, only its SHA-256     |

Access tokens are HS256 JWTs with `sub` (public account id), `uid`,
`sid` (session id) and `scope: store`. Refresh tokens are opaque and
**rotated on every use**: replaying an old refresh token invalidates
the session.

A permanent Store credential is never handed to the client, and no
token is ever stored inside `timekeeper.html`.

## Device identity

Each installation generates its own identity:

```
TK-DEVICE-7F82A1B44C9E0D13
```

Eight random bytes, hex, uppercase. Deliberately not a hardware
fingerprint: it can be revoked, reissued and forgotten, and it tells
the Store nothing about the machine.

An installation may register an ed25519 public key later for signed
requests. The private key never leaves the device.

## Session file

The CLI stores the session under the TimeKeeper state directory:

```
.timekeeper/store-auth.json
```

Written with owner-only permissions (0600). It contains the
installation id, the access token, the refresh token and the account
id. No password, no Store secret, no signing key.

## Freshness

Mutating requests carry `X-TK-Timestamp` and `X-TK-Nonce`. The
gateway rejects stale timestamps and, when nonce enforcement is on,
replayed nonces. This is a transport guarantee only — it never
replaces the entitlement check.

## Local usage

```bash
tk store status
tk store login you@example.com --password '<password>'
tk store catalogue
tk store install extension.memory-galaxy
tk store logout
```

`--password` can be replaced by `TIMEKEEPER_STORE_PASSWORD`. There is
no silent interactive prompt: echoing a password into terminal
scrollback is worse than requiring a flag.

`TIMEKEEPER_STORE_URL` selects the Store. Set
`TIMEKEEPER_STORE_STATE_DIR` to keep the session outside the
repository.

## Running the Store locally

The private `TimeKeeper-Store` repository ships a Node service with a
built-in SQLite driver for development, so the whole flow can be
exercised without a database server:

```bash
node scripts/genkeys.js
node scripts/seed.js
node server.js            # http://127.0.0.1:8787
TIMEKEEPER_STORE_URL=http://127.0.0.1:8787 tk store catalogue
```
