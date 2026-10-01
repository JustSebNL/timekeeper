#!/usr/bin/env bash
# Stable Hermes monitor for TimeKeeper Pulse attention.
# Runtime durations are intentionally removed so an unchanged overdue set does
# not wake a new agent run every five minutes.
set -euo pipefail

TK="${TK:-/mnt/d/dev/codebase/Dev/TimeKeeper/.timekeeper/app/bin/tk}"
pulse_output="$($TK pulse 2>/dev/null || true)"
attention="$({ printf '%s\n' "$pulse_output" | awk -F '\t' '
  $1 == "sprint_overdue" { print $1 "|" $2 "|" $3 "|" $7 }
' | LC_ALL=C sort; } || true)"

if [[ -z "$attention" ]]; then
  printf 'clear\n'
else
  # The overdue set can remain unchanged while an agent still needs several
  # wake cycles. Change only once per monitor interval so runtime durations do
  # not cause noisy duplicate runs, but a stable overdue set still reactivates
  # the agent instead of firing only once.
  now_epoch="${TIMEKEEPER_MONITOR_EPOCH:-$(date +%s)}"
  bucket=$((now_epoch / 300))
  printf 'timekeeper_attention bucket=%s\n%s\n' "$bucket" "$attention"
fi
