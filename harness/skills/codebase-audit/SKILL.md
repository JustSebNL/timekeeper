---
name: timekeeper-codebase-audit
description: Use when auditing an unfamiliar, unstable, or security-sensitive codebase.
---

# Codebase audit

1. Inventory entrypoints, storage, APIs, services, UI, dependencies, and tests.
2. Trace important flows end-to-end before proposing fixes.
3. Record each confirmed bug as a Task with severity, evidence, affected paths, reproduction, expected behavior, and verification.
4. Keep hypotheses separate from confirmed findings.
5. Split investigation, implementation, and verification into separate Sprints.
6. Run the focused regression before and after the fix, then update the Project event trail.

Audit first; do not turn a broad scan into an unbounded rewrite.
