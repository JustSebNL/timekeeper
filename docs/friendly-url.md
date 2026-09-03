# Friendly Local URLs: `timekeeper.local` and `api.timekeeper.local`

Status: implemented.

All code paths landed. The friendly URLs work end-to-end on hosts
where the proxy can bind port 80. On hosts where port 80 is
already taken (Windows in particular), the proxy logs a warning,
the canonical 1618 listener keeps working, and `tk doctor`
reports which endpoints are reachable.

Verified live:

- `timekeeper.exe -proxy-addr 127.0.0.1:80` starts the proxy
  listener alongside the canonical 1618 listener
- `tk open --ip` opens the dashboard in the default browser
- `tk doctor` probes all three endpoints and reports accurately
- `tk hosts status` reads the OS hosts file
- `tk hosts add` (when run with elevation) writes the entries
  idempotently
- `tk uninstall` strips the hosts entries
- `install.sh` calls `tk hosts add` after install
- `service-install.bat` and `service-install.sh` (both copies)
  pass `-proxy-addr 127.0.0.1:80` to NSSM / systemd
- README has a "Friendly URLs" section explaining the URLs, the
  proxy, the port-80 conflict, and the `TIMEKEEPER_PROXY_ADDR`
  override

## Goal

TimeKeeper is served at two human-friendly hostnames, no port in the URL:

- `http://timekeeper.local/` — the dashboard (browser)
- `http://api.timekeeper.local/` — the API (tools, agents, scripts)

The IP `127.0.0.1:1618` stays exposed as a fallback address for
diagnostics and for the small number of cases where the loopback
address is genuinely the right answer (containers, WSL-to-Windows
interop where `*.local` does not resolve, scripted health checks
that should not depend on the hosts file). It is no longer the
headline URL.

## Mechanism

Three pieces, all shipped by default, all owned by the project.

### 1. Hosts entries

`install.sh` writes two lines to the OS hosts file as part of the
normal install:

- Windows: `C:\Windows\System32\drivers\etc\hosts`
- Linux / WSL: `/etc/hosts`
- macOS: `/etc/hosts`

```
127.0.0.1 timekeeper.local
127.0.0.1 api.timekeeper.local
```

Properties:

- **Always written on install.** No flag, no opt-in. Every install
  gets the friendly URLs.
- **Idempotent.** Re-running `install.sh` is safe; each entry is
  checked for before being added.
- **Removed on uninstall.** `tk uninstall` strips both lines. We
  own the feature, we own its lifecycle.
- **mDNS is not a concern.** Hosts file is consulted before mDNS
  on every platform we target. `timekeeper.local` in the hosts
  file wins.

### 2. Reverse proxy: `127.0.0.1:80 → 127.0.0.1:1618`

TimeKeeper binds two listeners, both on loopback:

- `127.0.0.1:1618` — the API + dashboard, canonical port. The
  server's `mux` already routes `/` to the dashboard and
  `/api/...` to the API by path. The `Host` header is not used to
  gate routes; both `timekeeper.local` and `api.timekeeper.local`
  reach the same handler.
- `127.0.0.1:80` — the friendly-URL listener, a reverse proxy to
  the same handler. The browser hits this one.

Implementation:

- `timekeeper.exe` always starts the proxy. No flag to disable.
  Default proxy address: `127.0.0.1:80`.
- Override via env var `TIMEKEEPER_PROXY_ADDR` for users who
  genuinely need a different bind (rare; documented).
- ~20 lines of Go in `cmd/server/main.go` using
  `httputil.NewSingleHostReverseProxy`.
- Loopback-only check applied to the proxy address (same
  `validateServerConfig` pattern).

Service install:

- Windows NSSM service runs as SYSTEM — binding 80 is fine.
  `AppParameters` always includes `-proxy-addr 127.0.0.1:80`.
- Linux systemd user service binds 80 in the user namespace if
  available; the install log warns (and falls back to
  `127.0.0.1:8080` documented in README) if the user is
  unprivileged and cannot bind 80. The NSSM / root case is the
  common one.
- macOS: launchd plist uses the same flag.

### 3. Startup banner

The server prints a deliberate banner on start:

```
TimeKeeper is at:
  - http://timekeeper.local/         (dashboard)
  - http://api.timekeeper.local/     (API)
Canonical address (loopback, tools, diagnostics): http://127.0.0.1:1618/
```

Structured log (`internal/logging`) emits all three URLs in the
`timekeeper starting` and `timekeeper listening` events.

## CLI

### `tk open`

Opens the default browser. Default URL: `http://timekeeper.local/`.

Cross-platform:

- Windows: `rundll32 url.dll,FileProtocolHandler <url>`
- macOS: `open <url>`
- Linux: `xdg-open <url>`

`tk open --api` opens `http://api.timekeeper.local/` instead.
`tk open --ip` opens the canonical `http://127.0.0.1:1618/`.

`tk open` fails loudly (non-zero exit, "no browser available")
when no browser is registered. It never auto-starts the service
as a side effect.

### `tk uninstall`

Strips both hosts entries, stops the OS service, removes the
installed binaries from `.timekeeper/app/`. This is the only
place the hosts entries are removed; we wrote them, we own
them.

### `tk doctor`

Existing command, updated to verify:

- The two hosts entries resolve to `127.0.0.1`
- `timekeeper.local:80` and `api.timekeeper.local:80` both
  respond
- `127.0.0.1:1618` responds
- The OS service is running

## Real-world constraint: port 80 may already be in use

Some hosts (Windows in particular) reserve port 80 for system
services — HTTP.sys / IIS / Windows activation. PID 4 (`System`)
often holds `0.0.0.0:80` even when no application is visibly
using it. On those hosts, TimeKeeper's default `-proxy-addr
127.0.0.1:80` cannot bind.

The proxy startup handles this as a warning, not a fatal error:

```
[WARN] proxy listener could not bind 127.0.0.1:80: ...
       TimeKeeper canonical address is still http://127.0.0.1:1618/
       Set TIMEKEEPER_PROXY_ADDR to a free port, or free port 127.0.0.1:80, then restart.
```

The canonical 1618 listener keeps working. `tk doctor` reports
the proxy as failed. The user has three options:

1. **Free port 80.** Stop the service that holds it (often a
   Windows feature you can disable in "Turn Windows features on
   or off" → uncheck "Internet Information Services"). Not
   always possible on production / shared hosts.
2. **Pick a different port.** Set
   `TIMEKEEPER_PROXY_ADDR=127.0.0.1:8080` (or any unprivileged
   port). The friendly URL becomes `http://timekeeper.local:8080/`.
   The "no port in the URL" goal is sacrificed; the "friendly
   hostname" goal is preserved.
3. **Run without the proxy.** Pass `-proxy-addr ""` to disable
   the friendly-URL listener. Only the canonical 1618 address
   works. `timekeeper.local` is still in the hosts file but does
   not resolve to a listening port.

The installer must detect the port-80 conflict and present these
three options to the user rather than silently falling back.

## What stays the same

- `127.0.0.1:1618` remains a real listener. It is documented as
  the loopback / diagnostic / container address. Nothing about
  its behavior changes.
- The `mux` routing by path (`/` for dashboard, `/api/...` for
  the API) is unchanged. The proxy forwards to the same handler;
  no Host-header-based routing logic.
- `validateServerConfig` keeps rejecting non-numeric /
  non-loopback addresses on the canonical listener. The proxy
  address gets the same check.
- Pulse Guardian, `tk` CLI, and existing API clients all keep
  pointing at `127.0.0.1:1618` by default. They can be
  reconfigured to `http://api.timekeeper.local/` if the user
  prefers; the wire protocol is identical.
- SQLite database, web assets, and OS service install path are
  unchanged.

## What changes

| File | Change |
| --- | --- |
| `cmd/server/main.go` | Always start the proxy listener; update banner to the three URLs |
| `cmd/server/proxy.go` (new) | `httputil.NewSingleHostReverseProxy` wiring; loopback-only check |
| `internal/cli/cli.go` | Add `open` (with `--api`, `--ip`) and `uninstall` subcommands; extend `doctor` |
| `install.sh` | Write both hosts entries (idempotent); remove on `uninstall` |
| `scripts/service/service-install.bat` | Always pass `-proxy-addr 127.0.0.1:80` to NSSM |
| `scripts/service/service-install.sh` | Same for systemd `ExecStart` |
| `README.md` | Replace the loopback address in install / quick-start with the friendly URLs; keep the IP documented as the canonical address for tools and diagnostics |
| `web/index.html` | Footer shows the friendly URLs as primary; the IP as small-print |

## Out of scope for v1

- TLS. Loopback only; no cert.
- mDNS advertisement. Hosts file is enough.
- `tk open` on headless servers. Fails loudly with a clear message.
- A separate `timekeeper-proxy` binary. The proxy is one process
  with two listeners — the smallest mechanism that solves the
  problem.
- Host-based routing. Both hostnames reach the same handler; the
  server does not branch on `Host`. If we ever need to serve
  different content per hostname, that is a follow-up.
- CORS. The dashboard and API share an origin under
  `timekeeper.local` once the proxy is in place, so CORS does
  not come up for browser fetches. Direct API access from
  browser code is not a documented use case; document if it
  becomes one.

## Verification

After a fresh install and service start:

1. `curl http://timekeeper.local/health` returns `{"status":"ok"}`
2. `curl http://api.timekeeper.local/health` returns
   `{"status":"ok"}`
3. `curl http://127.0.0.1:1618/health` returns `{"status":"ok"}`
4. `curl http://timekeeper.local/api/v1/projects` returns the
   project list (same as
   `curl http://api.timekeeper.local/api/v1/projects`)
5. `tk open` opens the default browser to
   `http://timekeeper.local/`; `tk open --api` opens
   `http://api.timekeeper.local/`
6. Re-running `install.sh` does not duplicate either hosts
   entry
7. `tk uninstall` removes both hosts entries, stops the service,
   removes the binaries
8. `tk doctor` reports green for all four checks
9. A non-loopback proxy address is rejected at startup with a
   clear error
10. Pulse Guardian still posts to `http://127.0.0.1:1618` by
    default

## Open questions

- **`tk uninstall` scope.** Should it touch the SQLite database
  and the `.timekeeper/` state directory, or only what we wrote
  (binaries, service, hosts entries)? Lean: only what we wrote.
  State is the user's; they delete it themselves. Document
  this.
- **Linux port 80 unprivileged.** systemd user services cannot
  bind 80 without root. Document `sudo setcap
  'cap_net_bind_service=+ep' /path/to/timekeeper` as a one-shot
  step. No setuid wrapper.
- **macOS.** `brew services`-style install is a follow-up. v1
  targets Windows (NSSM) and Linux (root + setcap). Document
  the macOS manual setup, do not automate.
- **Configurability of the API hostname.** Should
  `api.timekeeper.local` be the only API hostname, or should
  the user be able to set their own (e.g. `tk.local`,
  `mycorp.local`)? Lean: fixed for v1 — `api.timekeeper.local`
  is part of the product. Override via env var
  `TIMEKEEPER_API_HOST` is a follow-up if anyone asks.
