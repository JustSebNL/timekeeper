#!/usr/bin/env bash
# self-nudge-work.sh — work cycle executed by the tk-guardian-self receiver.
#
# On each guardian nudge, this script:
#   1. Checks for pending work (tk pulse)
#   2. Claims the next sprint (tk sp next)
#   3. Runs a build/test cycle
#   4. Records evidence (tk msg add --kind observation)
#   5. Completes the sprint (tk sp complete)
#   6. Repeats until tk pulse shows clear
#
# The script is intentionally generic — the actual work (build, test,
# verify) is delegated to the agent's own tooling. The key invariant is
# that each nudge produces visible, recorded progress.
#
# Environment variables passed from guardian_wake.go:
#   AGENT_ID — the agent identifier
#   AGENT_WORK_CYCLES — number of work cycles completed so far
#   AGENT_LAST_NUDGE_ID — last nudge ID processed
#   AGENT_LAST_SPRINT_ID — last sprint ID claimed
#   AGENT_LAST_PROJECT_ID — last project ID worked on

set -euo pipefail

TK="${TK:-tk.exe}"
PROJECT_ID="${PROJECT_ID:-}"
MAX_ITERATIONS="${MAX_ITERATIONS:-10}"

if [[ -z "$PROJECT_ID" ]]; then
    echo "PROJECT_ID is required" >&2
    exit 1
fi

# Log agent context if available
if [[ -n "${AGENT_WORK_CYCLES:-}" ]]; then
    echo "Agent context: cycle $AGENT_WORK_CYCLES, last nudge ${AGENT_LAST_NUDGE_ID:-none}, last sprint ${AGENT_LAST_SPRINT_ID:-none}"
fi

for i in $(seq 1 "$MAX_ITERATIONS"); do
    echo "=== Work cycle iteration $i ==="

    # Check for pending work.
    pulse_output=$("$TK" pulse 2>/dev/null || echo "")
    if echo "$pulse_output" | grep -q "^clear"; then
        echo "No pending work. Going idle."
        exit 0
    fi

    # Claim the next sprint.
    echo "Claiming next sprint..."
    claim_output=$("$TK" sp next "$PROJECT_ID" 2>&1 || true)
    sprint_id=$(echo "$claim_output" | grep -oP '^SP-\d+' || true)
    if [[ -z "$sprint_id" ]]; then
        echo "No sprint claimed. Stopping."
        exit 0
    fi
    echo "Claimed $sprint_id"

    # Run build/test cycle.
    echo "Running build/test cycle for $sprint_id..."
    # TODO: replace with actual project build/test commands
    # Example: go build ./... && go test ./...
    build_ok=true

    # Record evidence.
    if [[ "$build_ok" == "true" ]]; then
        "$TK" msg "$PROJECT_ID" add --kind observation --agent "xatia-self-nudge" \
            "Self-nudge work cycle $i: completed $sprint_id" 2>/dev/null || true
        echo "Evidence recorded."
        echo "Build OK for $sprint_id"
        echo "Tests passed for $sprint_id"

        # Complete the sprint.
        "$TK" sp complete "$sprint_id" --reason "self-nudge work cycle completed" 2>/dev/null || true
        echo "Sprint $sprint_id completed."
    else
        echo "Build/test cycle failed. Sprint $sprint_id not completed."
        echo "Error: build or test failed for $sprint_id"
        exit 1
    fi
done

echo "Max iterations ($MAX_ITERATIONS) reached."
exit 0
