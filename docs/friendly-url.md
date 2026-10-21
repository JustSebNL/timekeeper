# Friendly local URLs

The standard local entry point for the dashboard, API, and agent workflows is `http://timekeeper.local/`. Use this friendly hostname in browser links, CLI examples, and agent configuration. If it does not work, use `tk doctor` and `tk hosts status` to diagnose resolution or proxy setup; do not substitute an implementation address in normal instructions.

## Addresses

- `http://timekeeper.local/` — the standard dashboard and local API entry point.
- `http://api.timekeeper.local/` — friendly API hostname on the same local listener.
- `http://vite.timekeeper.local:<port>/` — Vite development server alias; use the port printed by Vite. This is not a Core service URL.

The CLI uses `http://timekeeper.local/` as its normal API contact. Agents should use this hostname as their entry point or omit `--url` to use the default. WSL CLI requests map managed friendly hostnames at the transport layer, preserving the friendly Host header without requiring write permission to `/etc/hosts`.

## What has to work

The friendly names depend on two local pieces:

1. The operating-system hosts file contains the managed Core aliases (`timekeeper.local`, `api.timekeeper.local`, `guardian.local`) and the Vite development alias (`vite.timekeeper.local`), all directed to the local service host.
2. TimeKeeper's optional loopback proxy can bind its configured port (port 80 by default) and serves the Core handler for the Core aliases.

The Vite alias is only a hostname mapping: use `http://vite.timekeeper.local:<port>/` with the port reported by the Vite dev server. The bare `http://vite.timekeeper.local/` uses port 80 and therefore reaches the TimeKeeper Core proxy, not Vite. A portless Vite URL would require a separate host-based reverse-proxy route; this installer does not create one.

The installer attempts to add the friendly hosts-file entries. That step may require Administrator/root permission. `tk hosts status` checks the entries; `tk hosts add` and `tk hosts remove` manage them. The server and proxy remain local-only; do not expose either listener on a LAN or public interface.

## If the friendly URL does not open

1. Run `tk doctor` and check the managed host entries with `tk hosts status`.
2. If entries are missing, run `tk hosts add` from an elevated shell.
3. Check the proxy results from `tk doctor`. Port 80 may already be in use, especially on Windows. Do not stop or reconfigure another service unless you know who owns it.
4. Use `tk open` to try the standard friendly entry point after the configuration check.

If the proxy is configured on a non-default port, use the friendly URL reported by `tk doctor` or `/api/v1/connection-info`; it may include `:<port>`. On Linux/WSL, the source installer can select a custom proxy port or disable the proxy. These options are platform-specific; see [Installation](install.md) before changing service configuration.

A failed friendly-URL check does not by itself mean the TimeKeeper API or dashboard is unavailable. Use `tk doctor` to inspect the configured local service before changing system settings.

## Opening the dashboard

```text
tk open        # open the standard friendly dashboard URL
tk open --api  # open the friendly API hostname
```

`tk open` checks `/health` before launching the browser and does not start or restart the service. It uses `rundll32` on Windows, `open` on macOS, and `xdg-open` on Linux; a headless host without a browser launcher should use the URL directly. `tk doctor` is the first check when the service or URL is unavailable.

## Operator details and verification

The short troubleshooting path is `tk doctor`, `tk hosts status`, and `tk open`. The details below explain how the local aliases are managed and how to diagnose a proxy conflict.

### Hosts-file entries

The CLI manages a marked block in the operating-system hosts file:

| Platform | Hosts file |
| --- | --- |
| Windows | `%SystemRoot%\System32\drivers\etc\hosts` |
| Linux / WSL / macOS | `/etc/hosts` |

The current block maps `timekeeper.local`, `api.timekeeper.local`, `guardian.local`, and `vite.timekeeper.local` to `127.0.0.1`. The dashboard and API use the first two; `guardian.local` is used by Guardian, and `vite.timekeeper.local` is a development-only alias whose Vite port must be included. `tk hosts add` is safe to rerun; `tk hosts status` reports missing managed mappings; `tk hosts remove` removes only TimeKeeper's marked block and keeps unrelated lines. Editing the operating-system hosts file may require Administrator/root permission.

The installer attempts this setup. The Linux/WSL shell installer reports a failed write but can still leave a healthy canonical listener. The Windows service installer runs elevated and treats a failed host setup as an install failure; for an existing service, pass `-UpdateHosts` to refresh the managed entries, or run `tk hosts add` from an Administrator shell. In either case, check the actual result with `tk hosts status` rather than assuming the names were added.

### Listener and generated URLs

The canonical server handles both the dashboard and versioned API on loopback port 1618 by default. The optional friendly listener is also loopback-only and uses port 80 by default; it forwards to the same handler. The host header does not select a separate application.

TimeKeeper also holds a kernel-managed instance lock for the resolved SQLite database before opening it, and reserves the primary listener before migrations or Guardian startup. A second Core using the same database is refused even if it requests another port; a second Core attempting the same listener is refused before it opens its database. Locks release automatically when the process exits or crashes. Read-only backup mode does not take the runtime lock. Separate database plus separate listener remains available for isolated development/test instances.

The running server reports its current friendly URLs at `GET /api/v1/connection-info`, together with its stable database instance ID, API version, build identity, and active capability list. Those URLs may include a port when the configured proxy is not using port 80. `tk doctor` checks the configured API endpoint and the URLs returned by the running service. `tk open` opens the friendly dashboard or API hostname.

A browser's default HTTP port is 80, so `http://timekeeper.local/` works only if the host mapping and proxy are both ready. When a different port is advertised, use the complete URL reported by `tk doctor` or `/api/v1/connection-info`, for example `http://timekeeper.local:8080/`.

### Port 80 conflicts and supported choices

On native Linux, the shell service manager uses `ss` or `netstat` to check whether the configured loopback port is already listening. If port 80 is occupied and the install is interactive, it offers these choices:

1. Free port 80 and rerun the install.
2. Choose a different port; the friendly URL then includes `:<port>`.
3. Disable the proxy and intentionally give up the friendly browser entry point.

The shell installer persists the custom proxy-port choice and `--without-proxy` in `.timekeeper/app/INSTALLATION.env`; subsequent Linux service installs honor that choice. An explicit `TIMEKEEPER_PROXY_ADDR` takes precedence during service setup.

The probe returns unknown under WSL because Linux cannot see Windows-side listeners. The native Windows PowerShell installer also relies on the server's runtime bind check instead of this Linux prompt. If the friendly proxy cannot bind, the server reports the issue and `tk doctor` shows the affected entry point. Do not stop IIS, HTTP.sys, or another service unless you know who owns it.

### Checks you can run

With the service running, check the advertised contact first:

```bash
tk doctor
tk hosts status
curl -fsS http://timekeeper.local/health
```

If WSL cannot resolve the friendly hostname, check `tk hosts status` and repair the managed entries. Use `tk doctor` to check the local service and proxy; do not switch agent instructions or project links to another address.

### Scope and security

Friendly URLs do not add authentication, TLS, or remote access. The Core listener and optional proxy must remain local-only. Both friendly hostnames reach the same handler, so `/` serves the dashboard and `/api/...` serves the API regardless of which hostname was used. There is no separate API process or host-based access policy.

Earlier design notes described port 80 and host-file setup as unconditional, proposed a macOS launchd service, and said uninstall removed installed binaries. Those are not current operational instructions: host setup can fail, the proxy can be changed or disabled on supported installer paths, this repository has no automated launchd installer, and uninstall leaves `.timekeeper/` (including binaries and local data) in place. See the current [install guide](install.md) for platform-specific behavior.

## Managed host-block details and maintainer checks

### Managed hosts block

`tk hosts add` writes a sentinel-delimited TimeKeeper block for the managed friendly names. Use `tk hosts status` to inspect it and `tk hosts add` or `tk hosts remove` to manage it; manual edits are unnecessary.

`tk hosts status` checks all four managed names. The dashboard and API links in this guide are `timekeeper.local` and `api.timekeeper.local`; the helper also manages `guardian.local` and the Vite development alias. If the installer could not write the file, run `tk hosts add` from an elevated shell and check the result rather than retrying service installation.

### Maintainer smoke checklist

For a local change to URL or proxy behavior, verify the complete user path rather than only checking that a listener bound:

1. `tk doctor` reports local service health and the friendly entry-point status.
2. `tk hosts status` reports each managed mapping; rerunning `tk hosts add` does not duplicate the marked block.
3. `tk open` opens the friendly URL when its mapping and listener work.
4. The working friendly URL returns `{"status":"ok"}` from `/health`.
5. A busy/unavailable proxy is clearly reported, and configured friendly URLs include their port when needed.
6. A remote or wildcard listener address is rejected; the dashboard and API remain loopback-only.
7. `tk uninstall` removes the service and managed host block where permissions allow, but leaves the local database and installed app directory in place.

## Removing friendly URLs

`tk uninstall` removes the OS service and attempts to remove TimeKeeper's managed hosts-file entries. It leaves the `.timekeeper/` directory, including the installed binaries and local data. If the hosts-file update needs elevated permission, check `tk hosts status` and remove the entries from an elevated shell.
