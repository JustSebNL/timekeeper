#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
# TIMEKEEPER_FAILURE_ONLY_SERVICE_KICKER
set -Eeuo pipefail
umask 077

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
ADDR="${TIMEKEEPER_ADDR:-127.0.0.1:1618}"
HEALTH_URL="http://$ADDR/health"

# Healthy is the silent fast path. This script is suitable for a coarse
# scheduler/backstop; it does not poll continuously or restart healthy work.
health_payload="$(curl -fsS --max-time 4 "$HEALTH_URL" 2>/dev/null || true)"
case "$health_payload" in
  *'"status":"ok"'*)
    exit 0
    ;;
esac

# Prefer the installed service manager when a user service exists. If no
# supervisor is available, fall back to the repository launcher, which owns
# the loopback process recovery path.
if command -v systemctl >/dev/null 2>&1 && systemctl --user is-enabled timekeeper.service >/dev/null 2>&1; then
  systemctl --user restart timekeeper.service
  exit $?
fi

exec bash "$ROOT/launcher.sh" --force-restart --no-update
