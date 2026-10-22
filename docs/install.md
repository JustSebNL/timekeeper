# Install TimeKeeper

TimeKeeper builds from this checkout and stores runtime data in `.timekeeper/`.

## Before you start
- Go must be available to build.
- Back up `.timekeeper/` before replacing an installation.

## Linux / WSL
```bash
./install/install.sh
```
Builds and starts a systemd user service. Options: `--no-service`, `--with-port=PORT`, `--without-proxy`.

Manual start: `./.timekeeper/app/timekeeper`

## Windows
```powershell
.\install\install.bat
```
Builds from source and installs via Bend2Will. UAC approval required. `http://timekeeper.local/` is the entry point.

## macOS
Build-only: `./install/install.sh --no-service`. Start manually.

## Verify
```text
Linux/WSL: ./.timekeeper/app/tk doctor
Windows:   .\.timekeeper\app\bin\tk.exe doctor
```
Open: `http://timekeeper.local/`

## Service commands
```text
tk service status|start|stop|restart|logs|uninstall
```

## Uninstall
```text
tk uninstall
```
Removes the service but keeps `.timekeeper/` (database, binaries, logs).

## Troubleshooting
| Symptom | Fix |
|---|---|
| Go missing / build fails | Install Go, confirm `go version` |
| Service won't start (Windows) | Approve UAC. Set `TIMEKEEPER_BEND2WILL_ROOT` if Bend2Will not found. |
| Hostname doesn't open | `tk hosts status` then `tk doctor` |
| Port 80 in use | Select another proxy port |
| Server unhealthy | `tk doctor`, check service logs |

For API and agent details, see the [documentation index](README.md).
