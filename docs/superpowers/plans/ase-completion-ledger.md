# ASE completion ledger — plan: docs/superpowers/plans/2026-09-26-fase-identity.md

Tracking: P-10053 / T-10461 / ST-10465 / SP-10466.
Registry/bootstrap prior evidence: M-548. Existing dirty checkout retained by user request; no commits or deployment.

Pre-flight: bootstrap produces a permanent AgentID and one-way activity session;
claim/event consumers need a Core-validated actor, not arbitrary header labels.
Guardian currently consumes activity session as a raw runtime session, which
would rehash it into a new fingerprint.

Ruling: validate registered actor through Core and resume using the existing
opaque activity token, never expose the registry fingerprint — preserves
privacy and continuity; tokens remain attribution, not authentication.

Execution:
- [x] Atomic Sprint/parent Task claims, release, owner retries, anonymous bypass prevention.
- [x] Immutable event actors, monthly profile activity and safe clickable links.
- [x] Guardian resume handshake, original identity propagation, stale-work checks.
- [ ] Jev decision contract, full suite/build/review and source-only handoff.
