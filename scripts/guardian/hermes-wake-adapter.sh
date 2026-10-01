#!/usr/bin/env bash
# hermes-wake-adapter.sh — agent-side Pulse Guardian tunnel.
#
# TimeKeeper does not spawn a nested Hermes process. It triggers a supervised
# Hermes cron job through the local gateway and waits for that gateway to own
# the durable run. This crosses the WSL/agent sandbox boundary without
# inheriting the parent's session pipes or filesystem checkpoint locks.

set -euo pipefail

REMINDER="${1:-}"
if [[ -z "$REMINDER" ]]; then
    REMINDER="You are idling — please continue remaining work."
fi
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/../.." && pwd -P)"
STATE_DIR="${TIMEKEEPER_STATE_DIR:-$REPO_ROOT/.timekeeper/state}"
AGENT_ID="${AGENT_ID:-xatia}"
LOCK_DIR="$STATE_DIR/guardian-wake-$AGENT_ID.lock"
HERMES_BIN="${HERMES_BIN:-/home/oc/.local/bin/hermes}"
HERMES_PYTHON="${HERMES_PYTHON:-/home/oc/.hermes/hermes-agent/venv/bin/python}"
HERMES_WAKE_JOB_ID="${HERMES_WAKE_JOB_ID:-f307b233286f}"
HERMES_WAKE_TIMEOUT="${HERMES_WAKE_TIMEOUT:-1800}"

if [[ ! -x "$HERMES_BIN" ]]; then
    HERMES_BIN="$(command -v hermes || true)"
fi
if [[ -z "$HERMES_BIN" || ! -x "$HERMES_BIN" ]]; then
    printf 'Hermes wake tunnel: Hermes CLI not found for overlap check\n' >&2
    exit 1
fi

mkdir -p "$STATE_DIR"
acquire_lock() {
    if mkdir "$LOCK_DIR" 2>/dev/null; then
        printf '%s\n' "$$" > "$LOCK_DIR/pid"
        return 0
    fi

    # A live owner means another delivery already admitted this agent. Treat
    # this delivery as coalesced rather than failed; the existing worker owns
    # the work. A dead owner is stale state and may be removed safely.
    owner_pid=""
    if [[ -r "$LOCK_DIR/pid" ]]; then
        IFS= read -r owner_pid < "$LOCK_DIR/pid" || true
    fi
    if [[ "$owner_pid" =~ ^[0-9]+$ ]] && kill -0 "$owner_pid" 2>/dev/null; then
        printf 'Hermes wake adapter: wake already admitted for %s; coalescing duplicate\n' "$AGENT_ID"
        exit 75
    fi
    if [[ -z "$owner_pid" ]]; then
        # Empty legacy lock directories are safe to remove only when rmdir
        # confirms that no owner metadata was created concurrently.
        rmdir "$LOCK_DIR" 2>/dev/null || {
            printf 'Hermes wake adapter: lock exists but owner is unknown for %s\n' "$AGENT_ID" >&2
            exit 1
        }
    else
        rm -f "$LOCK_DIR/pid" 2>/dev/null || true
        rmdir "$LOCK_DIR" 2>/dev/null || {
            printf 'Hermes wake adapter: stale lock could not be cleared for %s\n' "$AGENT_ID" >&2
            exit 1
        }
    fi
    mkdir "$LOCK_DIR" 2>/dev/null || {
        printf 'Hermes wake adapter: another wake cycle is already running for %s\n' "$AGENT_ID" >&2
        exit 1
    }
    printf '%s\n' "$$" > "$LOCK_DIR/pid"
}
acquire_lock
trap 'rm -f "$LOCK_DIR/pid" 2>/dev/null || true; rmdir "$LOCK_DIR" 2>/dev/null || true' EXIT

# The cron job is the tunnel. Its prompt is intentionally durable and reads
# the pending TimeKeeper state itself; no command from the nudge is executed.
printf 'Hermes wake tunnel: requesting gateway queue for cron job %s (agent %s)\n' "$HERMES_WAKE_JOB_ID" "$AGENT_ID"
printf 'Hermes wake tunnel: reminder=%s\n' "$REMINDER"
latest_run="$($HERMES_BIN cron runs "$HERMES_WAKE_JOB_ID" --limit 1 2>/dev/null || true)"
if printf '%s\n' "$latest_run" | grep -Eq '[[:space:]]running[[:space:]]'; then
    printf 'Hermes wake tunnel: cron job %s is already running; coalescing duplicate\n' "$HERMES_WAKE_JOB_ID"
    exit 75
fi
HERMES_HOME="${HERMES_HOME:-/home/oc/.hermes}" \
HERMES_PYTHON_ROOT="${HERMES_PYTHON_ROOT:-/home/oc/.hermes/hermes-agent}" \
HERMES_BIN="$HERMES_BIN" \
HERMES_WAKE_JOB_ID="$HERMES_WAKE_JOB_ID" \
HERMES_WAKE_TIMEOUT="$HERMES_WAKE_TIMEOUT" \
  "$HERMES_PYTHON" "$REPO_ROOT/scripts/guardian/hermes-cron-tunnel.py"
