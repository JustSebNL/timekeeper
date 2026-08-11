#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GO_BIN="${TIMEKEEPER_GO:-go}"
INSTALLER="$ROOT/install.sh"

"$GO_BIN" version >/dev/null 2>&1 || {
  printf 'install e2e test requires Go. Set TIMEKEEPER_GO to a Go executable.\n' >&2
  exit 69
}

TMP="$(mktemp -d "$ROOT/.timekeeper/install-e2e.XXXXXX")"
SOURCE="$TMP/source"
DESTINATION="$TMP/installed"
PORT="$((18000 + RANDOM % 1000))"
BASE_URL="http://127.0.0.1:$PORT"
SERVER_PID=''
cleanup() {
  if [[ -n "$SERVER_PID" ]]; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" >/dev/null 2>&1 || true
  fi
  git -C "$ROOT" worktree remove --force "$SOURCE" >/dev/null 2>&1 || true
  rm -rf "$TMP"
}
trap cleanup EXIT

git -C "$ROOT" worktree add --detach "$SOURCE" origin/main >/dev/null
TIMEKEEPER_GO="$GO_BIN" "$INSTALLER" --source "$SOURCE" --destination "$DESTINATION" >/dev/null

test ! -e "$DESTINATION/state/timekeeper.db"
test -x "$DESTINATION/timekeeper"
test -x "$DESTINATION/tk"
test -s "$DESTINATION/INSTALLATION.env"

TIMEKEEPER_ADDR="127.0.0.1:$PORT" "$DESTINATION/timekeeper" >"$TMP/server.log" 2>&1 &
SERVER_PID="$!"
for _ in $(seq 1 40); do
  if "$DESTINATION/tk" --url "$BASE_URL" doctor >"$TMP/doctor.log" 2>&1; then
    break
  fi
  sleep 0.25
done

[[ -s "$TMP/doctor.log" ]] || {
  printf 'installed Time Keeper did not become reachable; server log follows:\n' >&2
  <"$TMP/server.log" cat >&2
  exit 1
}
grep -Fq 'Time Keeper is ready.' "$TMP/doctor.log" || {
  printf 'installed tk doctor did not report readiness; output follows:\n' >&2
  <"$TMP/doctor.log" cat >&2
  exit 1
}
test -s "$DESTINATION/state/timekeeper.db"
printf 'install-e2e=passed\n'
