# Install — TimeKeeper

This is the end-to-end install walkthrough. It covers a clean clone, the
friendly-URL setup, the OS service install, and the post-install checks.

If anything here disagrees with `install.sh` itself, the script wins —
the script is the source of truth; this document explains what it does
and why.

## 1. Prerequisites

- A clean TimeKeeper clone at the desired path (e.g.
  `D:\dev\codebase\dev\TimeKeeper` or `/mnt/d/dev/codebase/dev/TimeKeeper`)
- Git (the installer verifies the checkout origin and HEAD)
- Go available on PATH (or set `TIMEKEEPER_GO=/path/to/go` to override)
- Administrator / root for the friendly-URL hosts-file write and the
  NSSM service install. The installer will *not* ask for elevation;
  the user is expected to run it elevated.

## 2. Run the installer

From the repo root:

```text
./install.sh
```

What it does, in order:

1. Verifies the checkout origin is `git@github.com:JustSebNL/timekeeper.git`
   (or the HTTPS variant) and HEAD is at `origin/main`.
2. Verifies `go` is available.
3. Builds `timekeeper` (server), `tk` (CLI), and `guardian` into
   `.timekeeper/app/bin/` using `go build -trimpath`.
4. Stages the web assets into `.timekeeper/web/`.
5. Writes the launcher scripts (`.timekeeper/app/timekeeper` and
   `.timekeeper/app/tk`) and `INSTALLATION.env`.
6. Atomically swaps the new install into place, preserving any
   existing `.timekeeper/state/` and `.timekeeper/web/`.
7. Calls `tk hosts add` to write the friendly-URL entries
   (`timekeeper.local`, `api.timekeeper.local`) to the OS hosts
   file. This step is best-effort: if the user is not elevated,
   the install still completes and the failure is reported.

## 3. Verify the install

After `install.sh` returns:

```text
./.timekeeper/app/bin/tk doctor
```

You should see something like:

```text
[ok] http://127.0.0.1:1618/health: ok
[ok] http://timekeeper.local/health: ok
[ok] http://api.timekeeper.local/health: ok

Time Keeper is ready.
```

If the canonical address works but the friendly URLs do not, the
installer has not been run with elevation, or port 80 is already
in use (see [Friendly URLs](friendly-url.md#real-world-constraint-port-80-may-already-be-in-use)).

## 4. Install as an OS service

The service is the right primary recovery mechanism. NSSM on Windows,
systemd user-scope on Linux.

```text
./.timekeeper/app/bin/tk service install
```

This dispatches to `scripts/service/service-manager.sh` (or
`scripts/service/service-install.bat` from a Windows shell), which:

- **Probes the default proxy port (127.0.0.1:80) at install time.**
  On native Linux, if the port is in use, the install prompts for
  one of the three documented options and persists the choice to
  `INSTALLATION.env` so future re-installs remember it. On WSL the
  probe is a no-op (Linux ss does not see Windows-side listeners);
  the runtime bind inside timekeeper.exe is authoritative and
  `tk doctor` reports the conflict honestly. On native Windows
  (the .bat path) the probe is also a no-op today; the runtime
  warning + `tk doctor` cover it.
- Installs the service with delayed auto-start.
- Sets the service's `AppParameters` (NSSM) or `ExecStart` (systemd)
  to include `-proxy-addr <resolved-address>`, so the friendly-URL
  proxy starts with the service.
- Configures log rotation (10 MiB × 5), restart-on-failure with
  a 5s delay, and stdout/stderr piped to `.timekeeper/log/`.

The three-option prompt (only on native Linux):

```text
[service] Port 80 on 127.0.0.1 is already in use.
[service] This is the proxy address for the friendly URLs (timekeeper.local).
[service] Choose one:
[service]   1) Free port 80 on this host and re-run the install
[service]   2) Use a different port (you will see timekeeper.local:<port> in the URL)
[service]   3) Run without the friendly-URL proxy (only 127.0.0.1:1618 will work)
[service] Choose [1/2/3] (default 2):
```

The chosen value is persisted to `INSTALLATION.env` as
`TIMEKEEPER_PROXY_ADDR=<addr>` (option 2) or `TIMEKEEPER_PROXY_DISABLED=1`
(option 3). Subsequent runs of `tk service install` honor the
persisted choice.

Override the prompt entirely with an environment variable:

```text
TIMEKEEPER_PROXY_ADDR=127.0.0.1:8080 ./.timekeeper/app/bin/tk service install
```

Verify the service is running:

```text
./.timekeeper/app/bin/tk service status
```

## 5. Open the dashboard

The fastest path:

```text
./.timekeeper/app/bin/tk open
```

This launches the default browser at `http://timekeeper.local/`.
The browser does the rest. Bookmarks saved from there work across
sessions.

Other entry points:

- `tk open --api` — opens `http://api.timekeeper.local/` (the API
  surface, useful for inspecting JSON output in a browser).
- `tk open --ip` — opens `http://127.0.0.1:1618/` (the canonical
  address, the only one that always works regardless of hosts-file
  state or port 80 availability).

## 6. Uninstall

```text
./.timekeeper/app/bin/tk uninstall
```

This stops the OS service, removes the installed binaries, and
calls `tk hosts remove` to strip the friendly-URL entries from
the OS hosts file. The `.timekeeper/state/` and `.timekeeper/web/`
directories are left in place; remove them manually if you want
a fully clean uninstall.

## What the installer does NOT do

- It does not download code from the network. The build is local.
- It does not change your `PATH`.
- It does not start the server automatically. `tk service install`
  is the explicit step for that.
- It does not silently retry the hosts-file write in the background.
  If the install is not elevated, the user runs `tk hosts add`
  again from an elevated shell.

## What to do if the install fails

| Symptom | Likely cause | Fix |
| --- | --- | --- |
| `Time Keeper requires Go to build locally` | Go is not on PATH and `TIMEKEEPER_GO` is unset | Install Go or `export TIMEKEEPER_GO=/path/to/go` |
| `checkout origin must be git@github.com:JustSebNL/timekeeper.git` | Working in a fork or a non-canonical clone | `git remote set-url origin git@github.com:JustSebNL/timekeeper.git` |
| `could not write friendly-URL hosts entries` | Not elevated | Re-run from an Administrator shell (Windows) or with `sudo` (Linux) |
| `proxy listener could not bind 127.0.0.1:80` | Port 80 is already in use on the host | See [Friendly URLs — port 80 conflict](friendly-url.md#real-world-constraint-port-80-may-already-be-in-use) |
| `timekeeper.local: connection refused` | Hosts entry missing or proxy not bound | `tk doctor` will tell you which one; `tk hosts add` for the former, `TIMEKEEPER_PROXY_ADDR` for the latter |

## Open TODOs for the installer

These are the items the installer is *not* yet doing but should, in
priority order. **Status as of the latest sync:**

1. **Detect port 80 conflict and present the three-option choice**
   documented in `docs/friendly-url.md` ("Free port 80", "Pick a
   different port", "Run without the proxy"). **Status: shipped on
   native Linux only.** The probe uses `ss`/`netstat` and is reliable
   on native Linux; on WSL2 it is a no-op (the Linux netstat does
   not see Windows-side listeners); on the native Windows .bat path
   it is also a no-op today. On the platforms where the probe works,
   the prompt persists the choice to `INSTALLATION.env` so re-installs
   remember. Follow-ups: (a) add a PowerShell-based probe in
   `service-install.bat` for native Windows, (b) document a manual
   "use this port" override that the WSL .sh can read from
   `INSTALLATION.env` even when the probe is a no-op.

2. **Idempotent re-install of the OS service.** **Status: shipped.**
   `tk service install` now compares the desired service parameters
   (`AppParameters`, `AppDirectory`, `DisplayName`) against the live
   NSSM values; on a match it skips the remove + restart cycle and
   just ensures the service is running. On a mismatch (e.g. a new
   `TIMEKEEPER_PROXY_ADDR`) it removes and reinstalls as before. The
   live process is no longer interrupted on every refresh.

3. **Verify NSSM is present on Windows before promising the service
   install will work.** **Status: shipped.** `find_nssm` in
   `scripts/service/service-manager.sh` now prints a structured
   error that lists every location that was checked, the download
   URL, and the three ways to fix it (place at the standard path,
   set `TIMEKEEPER_NSSM`, or put on PATH).

4. **Add an `install.sh --with-port <addr>` flag.** **Status: shipped.**
   `./install.sh --with-port=127.0.0.1:8080` sets
   `TIMEKEEPER_PROXY_ADDR` and persists it; `./install.sh
   --without-proxy` disables the proxy listener and persists
   `TIMEKEEPER_PROXY_DISABLED=1`. The flags are documented in
   `install.sh --help`. The persisted values are honoured by
   `service-manager.sh` on the next `tk service install`.

5. **Document macOS** launchd install path in the README. **Status:
   shipped.** A `macOS (manual launchd setup)` subsection of the
   README walks through the four-step manual flow: build via
   `install.sh`, drop a LaunchAgent plist, `launchctl load -w` and
   `launchctl start`, then `launchctl unload` for removal. The
   plist is provided in full.

6. **Test the Linux port-80 unprivileged path with `setcap`.** **Status:
   shipped in `install.sh`.** When the user is root on Linux and
   `setcap` is on PATH, the install grants `cap_net_bind_service=+ep`
   to the timekeeper binary so the friendly-URL proxy can bind
   127.0.0.1:80 without the whole service running as root. Non-root
   users get a one-line instruction to run the same `setcap` command
   themselves; missing `setcap` (libcap2-bin not installed) is
   detected and reported.

7. **Verify the install path is part of `tk doctor`.** **Status:
   shipped.** `tk doctor` now reads `INSTALLATION.env` next to the
   binary and reports the install target, source commit (short hash),
   and the persisted proxy choice. Catches the case where the
   service is running an old build after a refresh.

## What ships vs. what's experimental

| Surface | Status |
| --- | --- |
| `./install.sh` builds + stages the app | ships |
| `tk hosts add` writes the friendly-URL entries | ships |
| Install-time port-80 probe + 3-option prompt (native Linux) | ships |
| `TIMEKEEPER_PROXY_ADDR` env override | ships |
| Persisted `INSTALLATION.env` choice for re-installs | ships |
| `./install.sh --with-port` / `--without-proxy` flags | ships |
| `setcap` step in `install.sh` (Linux unprivileged port 80) | ships |
| `tk service install` idempotent (no-op on matching params) | ships |
| `find_nssm` actionable error with all searched paths | ships |
| macOS launchd flow in README | ships |
| `tk doctor` install-commit + target + proxy choice line | ships |
| Install-time port-80 probe on WSL | not yet (TODO 1 follow-up) |
| Install-time port-80 probe on native Windows (.bat) | not yet (TODO 1 follow-up) |
