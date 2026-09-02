---
name: timekeeper-troubleshooting
description: Use when TimeKeeper or the current project workflow stops functioning.
---

# Troubleshooting and recovery

1. Run `tk doctor`.
2. If the service is unavailable, run `scripts/timekeeper-harness.sh` or the Windows `.bat` equivalent.
3. Re-check health and inspect `.timekeeper/log/` or service logs.
4. Record the incident and observed evidence in a TimeKeeper note.
5. Form one root-cause hypothesis.
6. Make one narrow fix.
7. Run the focused regression, then the broader suite.
8. Complete the Sprint only after reading back the recovery event.

The kick scripts are failure-only. Never restart a healthy service, and never hide a service failure with a UI-only success message.
