---
name: timekeeper-troubleshooting
description: Use when TimeKeeper or the current project workflow stops functioning.
---

# Troubleshooting and recovery

1. Run `tk doctor`.
2. If unavailable, follow the failure-only recovery procedure in `harness/SKILL.md`, then run `tk doctor` again.
3. Re-check health and inspect `/data/logs/` or service logs.
4. Record the incident and observed evidence in a TimeKeeper note.
5. Form one root-cause hypothesis.
6. Make one narrow fix.
7. Run the focused regression, then the broader suite.
8. Complete the Sprint only after reading back the recovery event.

## ASE-specific recovery

If a worker was nudged, do not treat callback delivery as resumed work. Check
`tk pulse` for the pending nudge and lease state, then inspect the adapter's
process/result. A callback is accepted only with the documented
`X-Timekeeper-Pulse-Accepted: v1` response; the nudge is acknowledged only
when the adapter has accepted a real work cycle. If the adapter cannot launch,
keep the nudge pending and record the blocker.

Never replace ASE with an unbounded shell loop, a second TimeKeeper server, or
an adapter that executes commands from the nudge payload. Use the stable agent
identity, durable checkpoint references, and the poll fallback documented in
`../../docs/anti-slack-engine.md`.

The kick scripts are failure-only. Never restart a healthy service, and never
hide a service failure with a UI-only success message.