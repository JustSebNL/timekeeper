# Friendly local URLs

TimeKeeper has a canonical loopback address and optional friendlier hostnames. These addresses are local conveniences; they do not make TimeKeeper a network service.

## Addresses

- `http://127.0.0.1:1618/` — canonical dashboard and API listener; use this when diagnosing a connection.
- `http://timekeeper.local/` — friendly dashboard hostname when the hosts-file entry and proxy route are available.
- `http://api.timekeeper.local/` — friendly API hostname when the same local setup is available.

Both friendly hostnames route to the same TimeKeeper handler. The dashboard is served at `/`; API routes are under `/api/`. The running service advertises the URLs it can currently use through `GET /api/v1/connection-info`; `tk open` uses that information when it can reach the service.

## What has to work

The friendly names depend on two local pieces:

1. The operating-system hosts file maps both names to `127.0.0.1`.
2. TimeKeeper's optional loopback proxy can bind its configured port (port 80 by default).

The installer attempts to add the hosts-file entries. That step may require Administrator/root permission. `tk hosts status` checks the entries; `tk hosts add` and `tk hosts remove` manage them. The server and proxy bind to loopback; do not expose either listener on a LAN or public interface.

## If the friendly URL does not open

1. Check the canonical address first: browse to `http://127.0.0.1:1618/` or run `tk doctor`.
2. Check the managed host entries with `tk hosts status`. If they are missing, run `tk hosts add` from an elevated shell.
3. Check the proxy results from `tk doctor`. Port 80 may already be in use, especially on Windows. Do not stop or reconfigure another service unless you know who owns it.
4. Use `tk open --ip` while diagnosing. The canonical loopback address does not depend on the friendly host entries.

If the proxy is configured on a non-default port, use the URL reported by `tk doctor` or `/api/v1/connection-info`; it may include `:<port>`. On Linux/WSL, the source installer supports `--with-port=127.0.0.1:<port>` and `--without-proxy`. These options are platform-specific; see [Installation](install.md) before changing service configuration.

A failed friendly-URL check does not by itself mean the TimeKeeper API or dashboard is unavailable. Confirm the canonical health result before changing any system service.

## Opening the dashboard

```text
tk open --ip   # open the canonical loopback address
tk open        # open the advertised friendly dashboard URL, when available
tk open --api  # open the advertised API hostname
```

`tk open` checks `/health` before launching the browser and does not start or restart the service. It uses `rundll32` on Windows, `open` on macOS, and `xdg-open` on Linux; a headless host without a browser launcher should use the URL directly. `tk doctor` is the first check when the service or URL is unavailable.

## Operator details and verification

The short troubleshooting path is `tk doctor`, `tk hosts status`, and `tk open --ip`. The details below are for users who need to understand how the local aliases are managed or diagnose a port conflict.

### Hosts-file entries

The CLI manages a marked block in the operating-system hosts file:

| Platform | Hosts file |
| --- | --- |
| Windows | `%SystemRoot%\System32\drivers\etc\hosts` |
| Linux / WSL / macOS | `/etc/hosts` |

The current block maps `timekeeper.local`, `api.timekeeper.local`, and `guardian.local` to `127.0.0.1`. The dashboard and API use the first two. `tk hosts add` is safe to rerun; `tk hosts status` reports missing managed mappings; `tk hosts remove` removes only TimeKeeper's marked block and keeps unrelated lines. Editing the operating-system hosts file may require Administrator/root permission.

The installer attempts this setup. The Linux/WSL shell installer reports a failed write but can still leave a healthy canonical listener. The Windows service installer runs elevated and treats a failed host setup as an install failure. In either case, check the actual result with `tk hosts status` rather than assuming the names were added.

### Listener and generated URLs

The canonical server handles both the dashboard and versioned API on loopback port 1618 by default. The optional friendly listener is also loopback-only and uses port 80 by default; it forwards to the same handler. The host header does not select a separate application.

The running server reports its current friendly URLs at `GET /api/v1/connection-info`. Those URLs may include a port when the configured proxy is not using port 80 or when the server advertises the canonical listener as the fallback. `tk doctor` checks the configured API endpoint and the URLs returned by the running service. `tk open` uses those URLs when it can read the connection information; `tk open --ip` uses the configured canonical base URL.

A browser's default HTTP port is 80, so `http://timekeeper.local/` works only if the host mapping and proxy are both ready. When a different port is advertised, use the complete URL reported by `tk doctor` or `/api/v1/connection-info`, for example `http://timekeeper.local:8080/`.

### Port 80 conflicts and supported choices

On native Linux, the shell service manager uses `ss` or `netstat` to check whether the configured loopback port is already listening. If port 80 is occupied and the install is interactive, it offers these choices:

1. Free port 80 and rerun the install.
2. Choose a different port; the friendly URL then includes `:<port>`.
3. Disable the proxy and use only the canonical address.

The shell installer persists `--with-port=127.0.0.1:<port>` and `--without-proxy` in `.timekeeper/app/INSTALLATION.env`; subsequent Linux service installs honor that choice. An explicit `TIMEKEEPER_PROXY_ADDR` takes precedence during service setup.

The probe returns unknown under WSL because Linux cannot see Windows-side listeners. The native Windows PowerShell installer also relies on the server's runtime bind check instead of this Linux prompt. If the proxy cannot bind, the server reports the issue and keeps the canonical listener available. Do not stop IIS, HTTP.sys, or another service unless you know who owns it.

### Checks you can run

With the service running, the canonical health endpoint should return `{"status":"ok"}`:

```bash
curl -fsS http://127.0.0.1:1618/health
tk doctor
tk hosts status
```

If the host mapping and default proxy are working, these local checks should also succeed:

```bash
curl -fsS http://timekeeper.local/health
curl -fsS http://api.timekeeper.local/health
```

For a custom port, use the exact host and port reported by TimeKeeper. A friendly-URL failure is not proof that the API is down; verify `http://127.0.0.1:1618/health` before changing services.

### Scope and security

Friendly URLs do not add authentication, TLS, or remote access. The canonical listener and optional proxy must remain on numeric loopback addresses. Both hostnames reach the same handler, so `/` serves the dashboard and `/api/...` serves the API regardless of which friendly hostname was used. There is no separate API process or host-based access policy.

Earlier design notes described port 80 and host-file setup as unconditional, proposed a macOS launchd service, and said uninstall removed installed binaries. Those are not current operational instructions: host setup can fail, the proxy can be changed or disabled on supported installer paths, this repository has no automated launchd installer, and uninstall leaves `.timekeeper/` (including binaries and local data) in place. See the current [install guide](install.md) for platform-specific behavior.

## Managed host-block details and maintainer checks

### Managed hosts block

`tk hosts add` writes a sentinel-delimited TimeKeeper block so a later `remove` can target only the entries owned by TimeKeeper:

```text
# >>> timekeeper begin
127.0.0.1 timekeeper.local
127.0.0.1 api.timekeeper.local
127.0.0.1 guardian.local
# <<< timekeeper end
```

`tk hosts status` checks all three managed names. The dashboard and API links in this guide are `timekeeper.local` and `api.timekeeper.local`; the helper also manages `guardian.local`. If the installer could not write the file, run `tk hosts add` from an elevated shell and check the result rather than retrying service installation.

### Maintainer smoke checklist

For a local change to URL or proxy behavior, verify the complete user path rather than only checking that a listener bound:

1. `tk doctor` reports the canonical `/health` result and any advertised friendly URLs.
2. `tk hosts status` reports each managed mapping; rerunning `tk hosts add` does not duplicate the marked block.
3. `tk open --ip` opens the canonical dashboard; `tk open` opens the advertised friendly URL when its mapping and listener work.
4. Both the canonical URL and working friendly URLs return `{"status":"ok"}` from `/health`.
5. A busy/unavailable proxy does not make the canonical listener fail, and configured URLs include their port when needed.
6. A remote or wildcard listener address is rejected; the dashboard and API remain loopback-only.
7. `tk uninstall` removes the service and managed host block where permissions allow, but leaves the local database and installed app directory in place.

## Removing friendly URLs

`tk uninstall` removes the OS service and attempts to remove TimeKeeper's managed hosts-file entries. It leaves the `.timekeeper/` directory, including the installed binaries and local data. If the hosts-file update needs elevated permission, check `tk hosts status` and remove the entries from an elevated shell.
