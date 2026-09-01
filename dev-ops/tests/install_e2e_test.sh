#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GO_BIN="${TIMEKEEPER_GO:-go}"
INSTALLER="$ROOT/install.sh"
EXPECTED_ORIGIN='git@github.com:JustSebNL/timekeeper.git'

"$GO_BIN" version >/dev/null 2>&1 || {
  printf 'install e2e test requires Go. Set TIMEKEEPER_GO to a Go executable.\n' >&2
  exit 69
}

TMP="$(mktemp -d "$ROOT/.timekeeper/install-e2e.XXXXXX")"
SOURCE="$TMP/source"
DESTINATION="$SOURCE/.timekeeper/app"
PORT="$((18000 + RANDOM % 1000))"
BASE_URL="http://127.0.0.1:$PORT"
SERVER_PID=''
cleanup() {
  if [[ -n "$SERVER_PID" ]]; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMP"
}
trap cleanup EXIT

git clone --no-hardlinks --local "$ROOT" "$SOURCE" >/dev/null
cp "$INSTALLER" "$SOURCE/install.sh"
git -C "$SOURCE" add install.sh
if ! git -C "$SOURCE" diff --cached --quiet -- install.sh; then
  git -C "$SOURCE" -c user.name='Time Keeper test' -c user.email='timekeeper-test@example.invalid' commit -m 'test current installer' >/dev/null
fi
git -C "$SOURCE" remote set-url origin "$EXPECTED_ORIGIN"
git -C "$SOURCE" update-ref refs/remotes/origin/main HEAD
(
  cd "$SOURCE"
  if ! TIMEKEEPER_GO="$GO_BIN" bash ./install.sh > "$TMP/install.log" 2>&1; then
    printf 'installed Time Keeper bootstrap failed; installer log follows:\n' >&2
    <"$TMP/install.log" cat >&2
    exit 1
  fi
)

test ! -e "$DESTINATION/state/timekeeper.db"
test -x "$DESTINATION/timekeeper"
test -x "$DESTINATION/tk"
test -s "$DESTINATION/INSTALLATION.env"
test -f "$(dirname "$DESTINATION")/web/index.html"
test -f "$(dirname "$DESTINATION")/web/vendor/bootstrap-5.3.8.min.css"

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
  printf 'installed server log follows:\n' >&2
  <"$TMP/server.log" cat >&2
  exit 1
}
test -s "$DESTINATION/state/timekeeper.db"

kill "$SERVER_PID" >/dev/null 2>&1 || true
wait "$SERVER_PID" >/dev/null 2>&1 || true
SERVER_PID=''
(
  cd "$SOURCE"
  TIMEKEEPER_GO="$GO_BIN" bash ./install.sh > "$TMP/refresh.log"
)
test -s "$DESTINATION/state/timekeeper.db"
grep -Fq 'preserving existing runtime state' "$TMP/refresh.log" || grep -Fq 'already installed and up to date' "$TMP/refresh.log"
printf 'install-e2e=passed\n'
