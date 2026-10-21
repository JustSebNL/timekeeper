# Local SQLite shadow copies

TimeKeeper Core retains one live SQLite authority. The shadow directory contains two recovery snapshots; neither is opened for normal reads or writes.

## Snapshot cycle

- The default directory is `recovery/sqlite-shadow/` beside the configured primary database. The default cadence is five minutes; use `-shadow-copy-dir <path>` to put snapshots on another volume and `-shadow-copy-interval <duration>` to change the cadence. Zero disables the worker; non-zero cadences under one minute are rejected.
- Each run performs a full `PRAGMA integrity_check` on the primary before snapshot work. If the result is not exactly `ok`, it skips the snapshot and leaves both slots and the manifest untouched.
- A healthy primary is snapshotted with SQLite `VACUUM INTO`; TimeKeeper never copies a live `.db` file or WAL as the snapshot mechanism.
- Core checks the staged candidate read-only, computes its SHA-256, flushes it, and promotes it into the inactive slot only after verification. `shadow-a.db` and `shadow-b.db` alternate; a failed snapshot cannot replace the last verified slot.
- The manifest records the active slot, hash, and time. If it is absent or invalid, Core checks both slots and can recover the newest valid one by file time. Each success and failure is logged.

The standalone `-backup-to` mode also refuses an unhealthy primary and verifies the resulting candidate before reporting success.

## Restore

Restore is explicit and offline. Stop the service first, then run the same server executable with its actual database and shadow paths, for example:

```text
timekeeper -db .timekeeper/timekeeper.db -shadow-copy-dir E:/Dev/codebase/Dev/TimeKeeper/.timekeeper/recovery/sqlite-shadow -restore-shadow-slot shadow-a.db -confirm-shadow-restore
```

The command requires the explicit confirmation flag, acquires the database instance lock, and reserves the configured loopback listener; it refuses if Core may still be running. It checks the selected shadow before touching the primary, stages and checks a copy beside the primary, preserves the previous `.db`, `-wal`, and `-shm` set in a unique `recovery-before-restore-*` directory, replaces the primary, and verifies the installed database. If verification fails, it attempts to restore the previous file set. It exits without restarting Core. Review the preserved-path and checksum output, then start Core explicitly.

A shadow copy is not automatically restored merely because Core reports a database error. The candidate may be stale or the primary may have newer transactions. Choose the slot only after reviewing its integrity and expected data; keep the preserved file set until the restored Core has been checked.

## This installation

The Windows installer prepares the shadow directory with explicit permissions for LocalService, SYSTEM, Administrators, and the installing user; inherited Everyone access is removed. Keep the directory on a separate volume when available, but note that this installation's E: target is a card-reader volume and therefore depends on that volume being mounted and healthy.

The local `.timekeeper/app/INSTALLATION.env` selects `E:/Dev/codebase/Dev/TimeKeeper/.timekeeper/recovery/sqlite-shadow` at five-minute intervals. The running Bend2Will service still has its old arguments and binary; the new flags take effect only after a controlled service update/restart. No live service update or restore has been performed.

Verify activation in the service log: a success entry names the slot, path, and SHA-256; a primary-integrity failure must say that the shadow was skipped. An `ok` health endpoint alone does not prove a shadow ran.
