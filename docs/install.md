# Install TimeKeeper

TimeKeeper is built from this repository checkout and stores its runtime data in the checkout's `.timekeeper/` directory. There is no downloadable application package in this repository. The Windows and Linux/WSL installers set up an OS service by default; macOS has a build-only path but no automated launchd service installer.

If you only need the commands, start with the [README](../README.md). This guide covers prerequisites, what each installer changes, and how to check or remove an installation.

## Before you start

- Use a checkout you trust. The Windows installer builds from the checked-out source. The Linux/WSL installer checks that the repository origin is the canonical TimeKeeper repository and that `HEAD` matches `origin/main`.
- Go must be available to build the app. The Linux/WSL installer also requires Git and the Go modules to already be available locally; it does not download application code or Go modules.
- The app, database, dashboard assets, and logs are kept under `.timekeeper/`. Back up data you need before replacing an installation or removing the checkout.
- The installer does not add the CLI to `PATH`. Use its checkout-local path in the commands below.

For a new Linux/WSL installation, the working tree must be clean. When an existing `.timekeeper/app` is present, the shell installer can refresh from a dirty checkout, but it builds the committed `HEAD`; uncommitted source changes are not included.

## Linux / WSL

From the repository root, run:

```bash
./install/install.sh
```

By default, this builds the server, CLI, and Guardian from the verified commit, stages the dashboard, attempts to add the friendly local hostnames, and installs and starts a systemd **user** service. The service setup needs a working systemd user manager; this may require enabling systemd in WSL.

Useful options are:

```bash
./install/install.sh --no-service
./install/install.sh --with-port=127.0.0.1:8080
./install/install.sh --without-proxy
```

- `--no-service` builds and stages the app without installing or starting a service.
- `--with-port` selects the loopback port for the optional friendly-URL proxy.
- `--without-proxy` disables that proxy; the canonical `127.0.0.1:1618` address remains available.

The proxy/host setup is optional. Writing `/etc/hosts` may require elevated permission; a failed host-file update is reported, but does not invalidate a healthy canonical listener. A port-80 conflict can also prevent the friendly URL from working. See [Friendly local URLs](friendly-url.md).

If you used `--no-service`, start the server manually from the repository root:

```bash
./.timekeeper/app/timekeeper
```

The installed CLI is available as `./.timekeeper/app/tk` (or the binary at `./.timekeeper/app/bin/tk`). To install a service later, run `./.timekeeper/app/tk service install` on a host with a working systemd user service.

## Windows

From the repository root, run in PowerShell or Command Prompt:

```powershell
.\install\install.bat
```

The installer builds the server and CLI, stages the dashboard, and uses Bend2Will's `obey` command to install the Windows service when a local Bend2Will checkout or `obey.exe` is available. In this workspace, the sibling `Bend2Will` repository is detected automatically. Elsewhere, set `TIMEKEEPER_BEND2WILL_ROOT` to that source checkout or `TIMEKEEPER_OBEY` to a trusted executable. The service install requests Administrator approval through UAC. NSSM remains a compatibility fallback when Bend2Will is unavailable; replacing an existing NSSM service requires the explicit `-MigrateFromNssm` option. The source changes do not alter the running service until an installer action is run and health-checked.

The CLI is `.\.timekeeper\app\bin\tk.exe`. The Windows installer builds the checked-out source directly; review the checkout before approving the elevated install. Friendly hostnames may still be unavailable if the proxy cannot bind its configured port. The canonical loopback address remains the fallback.

## macOS

The shell installer can build a macOS binary, but the service manager does not currently install a launchd service. An open support decision is recorded in the local TimeKeeper backlog; until it is resolved, the documented path is build-only. If you choose to build from source, use the shell installer with `--no-service`; the verified-checkout requirements above still apply. Start the resulting `.timekeeper/app/timekeeper` manually from the repository root and stop it in that terminal when finished. No launchd plist or automated macOS service setup is provided here.

## Verify and open TimeKeeper

After a default install, the service should already be running. Check its health with the CLI for your platform:

```text
Linux / WSL: ./.timekeeper/app/tk doctor
Windows:     .\.timekeeper\app\bin\tk.exe doctor
```

Open the dashboard using the canonical loopback address:

```text
Linux / WSL: ./.timekeeper/app/tk open --ip
Windows:     .\.timekeeper\app\bin\tk.exe open --ip
```

You can also browse directly to `http://127.0.0.1:1618/`. `tk open` uses the friendlier local hostname when it is available; `tk open --ip` avoids hosts-file and proxy setup. Check `tk hosts status` and `tk doctor` if a friendly URL does not work.

When `INSTALLATION.env` is present beside the CLI, `tk doctor` also reports the install target, source commit, and any saved proxy choice. If that metadata file is absent, its informational install check is skipped without failing the health check.

For ordinary work, use the dashboard or the CLI. The CLI and dashboard use the same local service and SQLite data.

## Installer behavior, in detail

The commands above are the quick path. These notes explain what the platform scripts actually do, including the differences that matter when troubleshooting.

### Linux / WSL shell installer

`install/install.sh` is a small repository-relative entrypoint to the root `install.sh`. On a new install the shell installer:

1. Verifies the canonical repository origin and requires `HEAD` to match `origin/main`.
2. Rejects a dirty working tree when no previous `.timekeeper/app` install exists. With an existing app it can proceed, but still archives and builds the committed `HEAD`; local edits are not included.
3. Uses `git archive` to stage that commit and runs Go with module downloading disabled. Build dependencies therefore need to be available locally.
4. Builds the `timekeeper`, `tk`, and `guardian` binaries, stages the dashboard, and writes local launchers plus `INSTALLATION.env` under `.timekeeper/app/`.
5. Refreshes the app directory while preserving its existing `state` directory. The SQLite database is stored outside the replaceable app directory at `.timekeeper/timekeeper.db`.
6. Attempts to add the managed local host entries. If the hosts-file write is denied, the canonical API can still work; the friendly names will not.
7. Installs and starts a systemd user service unless `--no-service` was selected.

The installer does not add the CLI to `PATH`, fetch application source, or turn on a remote listener. The `--with-port` and `--without-proxy` choices are saved in `INSTALLATION.env` and are used on later Linux service installs.

### Windows service installer

`install/install.bat` delegates to the repository's `install.bat`, which invokes `scripts/service/install-windows.ps1` with build and elevation enabled. It compiles `timekeeper.exe` and `tk.exe` from the checked-out source, builds `obey.exe` from the local Bend2Will source when available, stages the dashboard, and prepares `.timekeeper/`. Bend2Will registers the TimeKeeper service as `LocalService`, with delayed automatic startup and SCM restart-on-failure. Windows setup requests UAC approval instead of silently skipping elevated work.

The PowerShell installer looks for a sibling `Bend2Will` checkout by default. If it is elsewhere, set `TIMEKEEPER_BEND2WILL_ROOT` or pass `-Bend2WillRoot`; a trusted prebuilt binary can be supplied with `TIMEKEEPER_OBEY` or `-ObeyPath`. Preview the selected path and service settings without changing the service:

```powershell
.\scripts\service\install-windows.ps1 -Plan
```

An existing NSSM service is not silently replaced. First review the plan and confirm that the service belongs to this TimeKeeper checkout, then explicitly approve migration:

```powershell
.\install\install.bat -MigrateFromNssm
```

The installer checks that NSSM's configured application is this repository's `timekeeper.exe`, saves its settings, and only then replaces the service. If the Bend2Will service fails its ownership/health check, the installer attempts to restore the previous NSSM setup, replaced binaries, and the backed-up dashboard assets. During an existing-service migration, it leaves hosts-file entries unchanged unless `-UpdateHosts` is supplied and retains the legacy scheduled task by default; `-RetireLegacyLauncher` explicitly removes that task after health verification. Unknown service managers and services pointing at another executable are left untouched. The Windows service path does not perform the Linux installer's Git-origin/`origin/main` validation, so review the checkout before approving the build.

The Bend2Will service runs as `LocalService`, uses delayed automatic startup, and asks the SCM to restart its supervisor after failure with a five-second delay. Bend2Will captures supervisor and child output in `.timekeeper/log/service.log`, rotates at 10 MiB, and retains one `.1` backup. `tk service logs` shows recent output. The running service is not changed by source edits or by `-Plan`; migration happens only when an install action is explicitly run.

### Linux service, logs, and proxy choice

The Linux service is a systemd user unit under `~/.config/systemd/user/`. The installer enables user lingering when `loginctl` is available, then enables and starts the unit. The service manager writes logs to `/data/logs/` by default; if that directory is not writable, it falls back to `.timekeeper/log/` and prints a warning. `TIMEKEEPER_LOG_DIR` can select an explicitly provisioned log directory.

On native Linux, the service manager probes the default loopback proxy port. If it is already in use, an interactive install offers three choices: free the port and retry, select another port, or disable the friendly proxy. The selected port/disabled state is persisted. The port probe cannot see Windows-side listeners from WSL, and the native Windows PowerShell installer does not use that Linux prompt. In those cases, the server's bind result and `tk doctor` are the authoritative checks. A proxy failure does not make the canonical `127.0.0.1:1618` listener remote or unavailable.

On Linux, a privileged port may also need `CAP_NET_BIND_SERVICE`. When the shell installer runs as root and `setcap` is available, it grants that capability to the TimeKeeper server binary so the service can bind the proxy port without running the full server as root. If it cannot set the capability, the installer prints a warning; use a non-privileged proxy port or follow the printed `setcap` instruction if the no-port URL is important.

### What installation does not change

- It does not add `tk` to `PATH`; use the checkout-local CLI path.
- It does not enable LAN or public access. The service remains loopback-only and unauthenticated.
- It does not set up cloud sync or move the database out of `.timekeeper/`.
- It does not make the optional friendly hostnames a requirement for the dashboard or API.
- `--no-service` skips service installation only on the shell installer; it does not delete or reset existing Project data.

| Platform | Default installer result | Service behavior |
| --- | --- | --- |
| Linux | Verified-source build and staged local app | systemd user service installed and started |
| WSL | Same shell build, if systemd user services are available | systemd user service; Windows-side port conflicts are detected at runtime |
| Windows | Build from checked-out source and stage the dashboard; use Bend2Will when available, otherwise the NSSM compatibility fallback | Bend2Will or NSSM service is installed and health-checked after UAC approval; existing NSSM-to-Bend2Will migration requires `-MigrateFromNssm` |
| macOS | Verified-source build-only path with `--no-service` | No automated launchd service installer |

## Manage the service

Use the platform's installed CLI path with these commands:

```text
tk service status
tk service start
tk service stop
tk service restart
tk service logs
tk service uninstall
```

The installer also writes service logs under `.timekeeper/` or the configured Linux log directory. `tk service uninstall` removes the OS service; it does not delete the database or the rest of `.timekeeper/`.

If host-file setup was skipped or failed, `tk hosts status` shows the current entries. `tk hosts add` and `tk hosts remove` change the operating-system hosts file and may need Administrator/root permission.

## Uninstall and keep your data

Run:

```text
tk uninstall
```

This removes the OS service and attempts to remove TimeKeeper's managed friendly-host entries. It **leaves `.timekeeper/` in place**, including the database, installed binaries, dashboard assets, configuration, and logs. Keep or back up that directory if you want the data. Only remove it yourself after confirming you no longer need its contents; deleting it removes your local project history.

If the command could not edit the hosts file because it lacked permission, check `tk hosts status` and remove the entries from an elevated shell.

## Troubleshooting

| Symptom | What to check |
| --- | --- |
| Go is missing or the build fails | Install Go and confirm `go version`. On Linux/WSL, also make sure required Go modules are already cached. |
| Linux/WSL reports the wrong origin, revision, or a dirty checkout | The shell installer accepts a verified committed source revision, not arbitrary local edits. Check the remote and `HEAD`; do not expect uncommitted changes to be built. |
| Windows service setup does not start | Approve the UAC request. If Bend2Will cannot be found, set `TIMEKEEPER_BEND2WILL_ROOT` to its source checkout or `TIMEKEEPER_OBEY` to a trusted `obey.exe`; NSSM remains the fallback when available. Existing NSSM migration requires `-MigrateFromNssm`. Then run `tk service status` and `tk service logs`. |
| `127.0.0.1:1618` works but a friendly URL does not | Run `tk hosts status` and `tk doctor`. A missing hosts entry or unavailable proxy port affects the friendly name, not the canonical address. See [Friendly local URLs](friendly-url.md#if-the-friendly-url-does-not-open). |
| Port 80 is already in use | Do not stop an unrelated service just to get a shorter URL. Use `tk open --ip`, or choose a supported proxy option for your platform in the install guide. |
| The server is not healthy | Run `tk doctor`, then check service status and logs. Do not start a second server or database to work around an unhealthy install. |

For API, security, and agent integration details, see the [documentation index](README.md).
