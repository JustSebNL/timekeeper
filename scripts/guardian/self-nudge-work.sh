#!/usr/bin/env bash
# self-nudge-work.sh — default Pulse Guardian wake hook.
#
# This hook launches the configured agent-side adapter. The repository ships a
# conservative Hermes gateway implementation by default; deployments can
# override it with AGENT_WAKE_COMMAND. The adapter must establish real worker
# verified continuation before success is returned. In continuous mode the
# caller may invoke the adapter for several bounded cycles; the adapter must
# preserve the same semantics for every cycle.
#
# Set AGENT_WAKE_COMMAND to an executable adapter that accepts the reminder as
# its first argument when a different agent runtime is required.

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
REMINDER='You are idling — please continue remaining work.'
WAKE_COMMAND="${AGENT_WAKE_COMMAND:-$SCRIPT_DIR/hermes-wake-adapter.sh}"

# Runtime-neutral event metadata. Adapters translate this intent into their
# native resume/goal/message mechanism; these values are never shell syntax.
export GUARDIAN_EVENT="${GUARDIAN_EVENT:-guardian.continuation}"
export GUARDIAN_PRIORITY="${GUARDIAN_PRIORITY:-actionable}"
export GUARDIAN_OBJECTIVE="${GUARDIAN_OBJECTIVE:-continue already-authorized TimeKeeper work}"
export GUARDIAN_SCOPE="${GUARDIAN_SCOPE:-bound project/task/sprint from durable wake context}"

printf 'REMINDER: %s\n' "$REMINDER"

if [[ -z "$WAKE_COMMAND" ]]; then
    printf 'No AGENT_WAKE_COMMAND is configured; leaving the nudge pending.\n' >&2
    exit 1
fi

if [[ ! -x "$WAKE_COMMAND" ]]; then
    printf 'AGENT_WAKE_COMMAND is not executable: %s\n' "$WAKE_COMMAND" >&2
    exit 1
fi

"$WAKE_COMMAND" "$REMINDER"
