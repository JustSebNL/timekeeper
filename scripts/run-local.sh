#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
# PORTABLE_REPO_LOCAL_LAUNCHER
set -Eeuo pipefail
umask 077

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_BIN="${TIMEKEEPER_GO:-go}"
STATE_DIR="${TIMEKEEPER_STATE_DIR:-$ROOT/.timekeeper}"
ADDR="${TIMEKEEPER_ADDR:-127.0.0.1:1618}"
DB_PATH="${TIMEKEEPER_DB:-$STATE_DIR/timekeeper.db}"
UI_PATH="${TIMEKEEPER_UI:-$ROOT/web/timekeeper.html}"

if [[ ! -f "$UI_PATH" ]]; then
  printf 'Time Keeper dashboard not found: %s\n' "$UI_PATH" >&2
  exit 66
fi
if ! "$GO_BIN" version >/dev/null 2>&1; then
  printf 'Time Keeper requires Go to build locally. Set TIMEKEEPER_GO to a Go executable.\n' >&2
  exit 69
fi

mkdir -p "$STATE_DIR"
chmod 700 "$STATE_DIR"
mkdir -p "$STATE_DIR/bin"
goos="$("$GO_BIN" env GOOS)"
binary="$STATE_DIR/bin/timekeeper"
[[ "$goos" == "windows" ]] && binary+=".exe"
build_output="$binary"
runtime_db="$DB_PATH"
runtime_ui="$UI_PATH"
if [[ "$goos" == "windows" ]] && command -v wslpath >/dev/null 2>&1; then
  build_output="$(wslpath -w "$binary")"
  runtime_db="$(wslpath -w "$DB_PATH")"
  runtime_ui="$(wslpath -w "$UI_PATH")"
fi

(
  cd "$ROOT"
  "$GO_BIN" build -o "$build_output" ./cmd/server
)

printf 'Time Keeper local state: %s\n' "$STATE_DIR"
printf 'Time Keeper dashboard: http://%s/\n' "$ADDR"
exec "$binary" -addr "$ADDR" -db "$runtime_db" -ui "$runtime_ui"
