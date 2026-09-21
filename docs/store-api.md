# TimeKeeper Store API (v1)

The protocol between TimeKeeper Core and TimeKeeper-Store. Publishing
this document does not publish the Store: the server implementation
lives in the private `TimeKeeper-Store` repository.

Base URL in production is the Cloudflare Worker gateway. The origin is
never contacted directly.

```
https://api.timekeeper.example/v1
```

Local development may point `TIMEKEEPER_STORE_URL` at a loopback Store
(for example `http://127.0.0.1:8787`). Non-HTTPS is refused except for
loopback.

## Client rules

Every request carries:

| Header                  | When           | Purpose                        |
| ----------------------- | -------------- | ------------------------------ |
| `X-TK-Installation-Id`  | always         | `TK-DEVICE-…` install identity |
| `X-TK-Timestamp`        | mutating calls | unix seconds, anti-replay      |
| `X-TK-Nonce`            | mutating calls | one-time value                 |
| `Authorization: Bearer` | protected      | short-lived access token       |

The gateway rejects mutating requests without a timestamp, rejects a
timestamp outside ±300 seconds, and rejects any route not on its
allowlist. Only `application/json` bodies are accepted.

## Versioning

`GET /v1/meta/versions` returns the versions the Store accepts. The
client sends `store_api = v1`; the Store may accept `v1, v2`. An older
TimeKeeper keeps working until its version is retired.

## Routes

### Public

| Method | Route                  | Purpose                          |
| ------ | ---------------------- | -------------------------------- |
| GET    | `/health`              | liveness                         |
| GET    | `/v1/meta/public-key`  | ed25519 package verification key |
| GET    | `/v1/meta/versions`    | accepted API versions            |
| POST   | `/v1/auth/register`    | create account                   |
| POST   | `/v1/auth/login`       | authenticate                     |
| POST   | `/v1/auth/refresh`     | rotate credentials               |
| POST   | `/v1/auth/logout`      | revoke session                   |

### Authenticated

| Method | Route                          | Purpose                    |
| ------ | ------------------------------ | -------------------------- |
| GET    | `/v1/account`                  | account summary            |
| GET    | `/v1/account/devices`          | registered installations   |
| POST   | `/v1/account/devices`          | register an installation   |
| GET    | `/v1/account/entitlements`     | entitlements for this user |
| GET    | `/v1/store/extensions`         | catalogue                  |
| GET    | `/v1/store/extensions/{id}`    | extension detail           |
| POST   | `/v1/store/claim`              | claim a free extension     |
| POST   | `/v1/store/install`            | install request            |
| POST   | `/v1/download/request`         | request a download ticket  |
| GET    | `/v1/download/{ticket}`        | stream the package         |

Reserved, not implemented yet: `/v1/checkout/create`,
`/v1/billing/subscriptions`, `/v1/billing/webhook`.

## Errors

Every error is JSON:

```json
{ "error": "not_entitled", "message": "This account does not have access to this extension." }
```

| Status | Codes                                                                 |
| ------ | --------------------------------------------------------------------- |
| 400    | `invalid_request`, `invalid_json`, `missing_timestamp`, `stale_request` |
| 401    | `unauthenticated`, `invalid_token`, `invalid_credentials`, `session_revoked`, `account_unavailable` |
| 402    | `payment_required`                                                     |
| 403    | `not_entitled`, `device_revoked`                                       |
| 404    | `not_found`, `unknown_product`, `unknown_version`, `invalid_ticket`    |
| 409    | `email_taken`, `device_conflict`, `replayed_request`                   |
| 410    | `package_missing`                                                      |
| 413    | `payload_too_large`                                                    |
| 415    | `unsupported_content_type`                                             |
| 429    | `rate_limited`                                                         |

Unknown JSON fields are rejected with `400 invalid_request`. A client
that adds `{"owned": true}` never reaches business logic.

## Authority

The client requests. The server decides. See `docs/auth-protocol.md`.
