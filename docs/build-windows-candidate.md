# Build a Windows candidate without installing

Status: build-only helper. It does not deploy or publish a TimeKeeper release.

Use this when you want to test the current checkout with Windows Go, especially when WSL can see `go.exe` but cannot launch Windows executables. From the checkout containing the source changes, run:

```text
dev-ops\build-windows-candidate.bat
```

The batch helper is intentionally stored in the Git-ignored `dev-ops/` workspace; it is local build support, not a product release script.

The script resolves the repository root from its own location and builds the local working tree at that path, including uncommitted local files. It does not fetch from or push to a remote repository; `GOPROXY=off` also prevents Go module downloads. The script requires the supported Windows Go toolchain (`%ProgramFiles%\Go\bin\go.exe` or `go.exe` on `PATH`) and cached modules. It runs `go test -count=1 ./...` and `go vet ./...` with `TIMEKEEPER_STORE_STATE_DIR` isolated under the candidate output directory, builds `tk.exe` and `timekeeper.exe`, then smoke-tests `tk version`, help output, and the local harness catalog.

Candidate binaries and isolated test state are left under `dev-ops/builds/candidate-*` inside the checkout. The complete build log, source revision/status, Go build metadata, and CLI smoke outputs go under `dev-ops/logs/`, beside the batch file. Both directories are Git-ignored local build artifacts. Because the helper only writes there, it does not need to stop the running service. It does not overwrite `.timekeeper/app/bin`, start or stop a Core/service, connect to the live API, touch SQLite or hosts files, request UAC, change tracked source, bump `VERSION`, or publish anything. Before a separate install/swap replaces installed binaries, stop TimeKeeper through its supported service manager and verify it is stopped before copying; restart only after replacement, then verify build identity.

Run the batch file from the checkout containing the intended source; a stale mirror builds that mirror. If a module is missing from the local Go cache, the offline build stops rather than fetching it. That error is useful evidence; do not switch to the service installer as a workaround. `install.bat` is a separate install/deploy workflow and may change the Windows service.
