#!/usr/bin/env bash
# Reference adapter for non-Hermes agent runtimes.
# Copy this file, replace submit-wake, and set AGENT_WAKE_COMMAND to its path.
set -euo pipefail

REMINDER="${1:-TimeKeeper requested a worker wake.}"
AGENT_ID="${AGENT_ID:?AGENT_ID is required}"
NUDGE_ID="${AGENT_LAST_NUDGE_ID:-unknown}"
RUNTIME="${AGENT_RUNTIME_SUBMIT:?Set AGENT_RUNTIME_SUBMIT to the supervisor command}"
LOCK_ROOT="${XDG_RUNTIME_DIR:-${TMPDIR:-/tmp}}"
LOCK_DIR="$LOCK_ROOT/timekeeper-wake-$AGENT_ID.lock"

if ! mkdir "$LOCK_DIR" 2>/dev/null; then
    printf 'agent wake adapter: %s already has an active execution\n' "$AGENT_ID" >&2
    exit 75
fi
trap 'rmdir "$LOCK_DIR" 2>/dev/null || true' EXIT

# The supervisor must deduplicate AGENT_ID + NUDGE_ID and return success only
# after a bounded worker step has been verified and recorded. REMINDER is data;
# never eval it or concatenate it into a shell command.
"$RUNTIME" \
    --agent "$AGENT_ID" \
    --nudge "$NUDGE_ID" \
    --reason "$REMINDER"
