---
name: timekeeper-service-recovery
description: Use when recovering TimeKeeper service availability safely.
---

# Service recovery

1. Run `tk doctor` first. If the service is healthy, do not restart it.
2. Capture the bounded failure evidence: the failing health check, service
   status, and the relevant `/data/logs/` entry when available.
3. Invoke exactly one platform recovery kick:
   - Linux/WSL: `bash scripts/service/kick-server.sh`
   - Windows: `scripts\\service\\kick-server.bat`
4. Run `tk doctor` once more. The kick is a failure-only recovery action, not a
   timer, poller, or second server.
5. Verify both the canonical API (`127.0.0.1:1618`) and the friendly health
   routes when configured. Health must be checked after the recovery action.
6. If the second check fails, stop retrying, inspect service logs, and record a
   bounded blocker in the owning Task/Sprint. Do not bypass the durable record.

## Ownership and safety

- Keep TimeKeeper Core on its canonical listener; never launch a duplicate
  server or database to hide an unavailable service.
- Keep the agent-side Guardian consumer separate from the Core service when it
  needs the interactive runtime profile.
- Do not infer recovery from process silence or terminal output. Use health,
  service status, logs, and a real API response.
- Preserve the original failure evidence and distinguish an old process still
  serving traffic from a newly verified binary or route.
