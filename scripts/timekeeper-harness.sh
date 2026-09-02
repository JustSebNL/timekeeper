#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
# TIMEKEEPER_AGENT_HARNESS_HEALTH_GATE
set -Eeuo pipefail
umask 077

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
ADDR="${TIMEKEEPER_ADDR:-127.0.0.1:1618}"
HEALTH_URL="http://$ADDR/health"
CLI="$ROOT/.timekeeper/app/tk"

healthy() {
  local payload
  payload="$(curl -fsS --max-time 4 "$HEALTH_URL" 2>/dev/null || true)"
  [[ "$payload" == *'"status":"ok"'* ]]
}

if healthy; then
  printf 'TimeKeeper healthy on %s\n' "$ADDR"
  exit 0
fi

printf 'TimeKeeper is unavailable; invoking failure-only recovery.\n' >&2
bash "$ROOT/scripts/service/kick-server.sh"
for _ in $(seq 1 10); do
  if healthy; then
    printf 'TimeKeeper recovered on %s\n' "$ADDR"
    exit 0
  fi
  sleep 1
done

printf 'TimeKeeper remains unavailable on %s. Inspect .timekeeper/log/ and run tk doctor when recovered.\n' "$ADDR" >&2
exit 1
