#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
# launcher.sh — TimeKeeper resilient launcher
#   - checks the canonical health endpoint before touching any process
#   - only recovers/starts when TK is absent or unhealthy
#   - pulls repo updates + rebuilds, but only if last check > 24h ago
#   - starts a fresh TK instance on the canonical port (127.0.0.1:1618)
#
# Usage:  bash launcher.sh [--force-update] [--no-update]
#   --force-update  ignore the 24h gate and update now
#   --no-update     never pull/build (just restart)
#
set -u

REPO="/mnt/d/dev/codebase/dev/TimeKeeper"
BIN="$REPO/.timekeeper/app/bin/timekeeper"
DB="$REPO/.timekeeper/timekeeper.db"
UI="$REPO/.timekeeper/web/index.html"
ADDR="127.0.0.1:1618"
PORT="1618"
UPDATE_INTERVAL_HOURS=24
STAMP="$REPO/.timekeeper/.last_update"
LOG="$REPO/.timekeeper/launcher.log"

mkdir -p "$(dirname "$LOG")"
log() { echo "$(date '+%Y-%m-%d %H:%M:%S') $*" | tee -a "$LOG"; }

# Serialize scheduler/manual invocations. The health probe is intentionally
# retained below, but this lock closes the race where two launchers probe the
# same moment and both decide they should start/restart TK.
LOCK="$REPO/.timekeeper/launcher.lock"
exec 9>"$LOCK"
if ! flock -n 9; then
  log "launcher already active — exiting without opening another TK start"
  exit 0
fi

cd "$REPO" || { log "FATAL: cannot cd to $REPO"; exit 1; }

FORCE_UPDATE=0
NO_UPDATE=0
FORCE_RESTART=0
for a in "$@"; do
  case "$a" in
    --force-update) FORCE_UPDATE=1 ;;
    --no-update) NO_UPDATE=1 ;;
    --force-restart) FORCE_RESTART=1 ;;
  esac
done

# ─── 1b. Idempotent health gate ─────────────────────────────────────────────
# A healthy instance is authoritative: do not kill it, rebuild it, or start a
# second instance. Use the machine-readable health endpoint and validate its
# payload; plain curl success is insufficient because HTTP 500 otherwise exits
# with status 0.
is_healthy() {
  local health_payload
  health_payload=$(curl -fsS --max-time 4 "http://$ADDR/health" 2>/dev/null) || return 1
  case "$health_payload" in
    *'"status":"ok"'*) return 0 ;;
    *) return 1 ;;
  esac
}

if [ "$FORCE_RESTART" -eq 0 ] && is_healthy; then
  log "TK already healthy on :$PORT — nothing to do"
  exit 0
fi

if [ "$FORCE_RESTART" -eq 1 ]; then
  log "explicit --force-restart requested — recovering TK despite health state"
else
  log "TK health check failed — entering recovery/start path"
fi

# ─── 1. Recover only the unhealthy canonical instance ───────────────────────
# Do not touch the legacy port here: a failure on :1618 is not evidence that a
# process on :1621 is broken.
kill_port() {
  local port="$1"
  # WSL-side holders
  local pids
  pids=$( (ss -ltnp 2>/dev/null || netstat -ltnp 2>/dev/null) | grep ":$port " | grep -oE 'pid=[0-9]+' | cut -d= -f2 | sort -u)
  for pid in $pids; do
    log "kill WSL pid $pid on :$port"
    kill -9 "$pid" 2>/dev/null
  done
  # Windows-side holders (TK binary sometimes launched from Windows)
  if command -v cmd.exe >/dev/null 2>&1; then
    local wps
    wps=$(cmd.exe /c "netstat -ano | findstr :$port " 2>/dev/null | grep LISTENING | awk '{print $5}' | sort -u)
    for wpid in $wps; do
      [ -n "$wpid" ] || continue
      log "kill Windows pid $wpid on :$port"
      cmd.exe /c "taskkill /F /PID $wpid" >/dev/null 2>&1
    done
  fi
}
kill_port "$PORT"

# ─── 2. Update (rate-limited to 24h) ────────────────────────────────────────
do_update=0
if [ "$NO_UPDATE" -eq 1 ]; then
  log "update skipped (--no-update)"
elif [ "$FORCE_UPDATE" -eq 1 ]; then
  do_update=1
elif [ ! -f "$STAMP" ]; then
  do_update=1
else
  last=$(stat -c %Y "$STAMP" 2>/dev/null || echo 0)
  now=$(date +%s)
  if [ $(( (now - last) / 3600 )) -ge "$UPDATE_INTERVAL_HOURS" ]; then
    do_update=1
  fi
fi

if [ "$do_update" -eq 1 ]; then
  log "update window open — git pull"
  before=$(git rev-parse HEAD 2>/dev/null || echo none)
  if git pull --ff-only 2>&1 | tee -a "$LOG"; then
    after=$(git rev-parse HEAD 2>/dev/null || echo none)
    if [ "$before" != "$after" ]; then
      log "remote changes pulled ($before -> $after)"
      if command -v go >/dev/null 2>&1; then
        log "rebuilding via install.sh"
        bash "$REPO/install.sh" 2>&1 | tail -20 | tee -a "$LOG" || log "WARN: install.sh failed; using existing binary"
      else
        log "Go not installed — cannot rebuild; using existing prebuilt binary (local edits will NOT be compiled)"
      fi
    else
      log "already up to date — no rebuild"
    fi
  else
    log "WARN: git pull failed; continuing with current binary"
  fi
  date +%s > "$STAMP"
else
  log "update skipped (last check < ${UPDATE_INTERVAL_HOURS}h ago)"
fi

# ─── 3. Start fresh TK on 1618 ──────────────────────────────────────────────
[ -x "$BIN" ] || { log "FATAL: $BIN not executable"; exit 1; }
# NOTE: the TK binary is Windows-cross-compiled and mangles absolute /mnt/d/
# paths into D:\mnt\d\... — so it MUST be launched with RELATIVE paths from
# the repo root (this is how the healthy :1621 instance runs).
log "starting TK on :$PORT (relative paths from $REPO)"
setsid nohup "$BIN" -addr "$ADDR" -db .timekeeper/timekeeper.db -ui .timekeeper/web/index.html -pulse-guardian-interval 5m \
  >> "$LOG" 2>&1 &

# ─── 4. Verify it came up ───────────────────────────────────────────────────
# The TK binary binds in the Windows socket namespace (WSL ss/netstat can't
# see it), so verify with the same machine-readable health predicate.
ready=0
for _ in $(seq 1 10); do
  if is_healthy; then
    ready=1
    break
  fi
  sleep 1
done
if [ "$ready" -eq 1 ]; then
  log "OK: TK healthy on :$PORT"
else
  log "WARN: TK health check still failing on :$PORT after 10s — check $LOG"
fi
