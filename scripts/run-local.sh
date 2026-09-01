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
UI_PATH="${TIMEKEEPER_UI:-$ROOT/.timekeeper/web/timekeeper.html}"
# The Guardian is a required backbone service: it evaluates registered local
# recovery leases on a bounded interval. Empty disables it (for explicit opt-out
# via TIMEKEEPER_PULSE_GUARDIAN_INTERVAL=""). Defaults to 5m.
PULSE_GUARDIAN_INTERVAL="${TIMEKEEPER_PULSE_GUARDIAN_INTERVAL:-5m}"
# When the Guardian is enabled, also start the repo-local recovery receiver that
# accepts recover_attention signals on loopback and records durable artifacts.
GUARDIAN_RECEIVER_ADDR="${TIMEKEEPER_GUARDIAN_RECEIVER_ADDR:-127.0.0.1:1619}"
GUARDIAN_RECEIVER_AGENT="${TIMEKEEPER_GUARDIAN_RECEIVER_AGENT:-xatia}"

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
cli_binary="$STATE_DIR/bin/tk"
[[ "$goos" == "windows" ]] && binary+=".exe" && cli_binary+=".exe"
build_output="$binary"
cli_build_output="$cli_binary"
runtime_db="$DB_PATH"
runtime_ui="$UI_PATH"
if [[ "$goos" == "windows" ]] && command -v wslpath >/dev/null 2>&1; then
  build_output="$(wslpath -w "$binary")"
  cli_build_output="$(wslpath -w "$cli_binary")"
  runtime_db="$(wslpath -w "$DB_PATH")"
  runtime_ui="$(wslpath -w "$UI_PATH")"
fi

(
  cd "$ROOT"
  "$GO_BIN" build -o "$build_output" ./cmd/server
  "$GO_BIN" build -o "$cli_build_output" ./cmd/tk
  if [[ -n "$PULSE_GUARDIAN_INTERVAL" && ! -x "$STATE_DIR/bin/guardian" ]]; then
    "$GO_BIN" build -o "$STATE_DIR/bin/guardian" ./cmd/guardian
  fi
)

printf 'Time Keeper local state: %s\n' "$STATE_DIR"
printf 'Time Keeper dashboard: http://%s/\n' "$ADDR"
server_args=( -addr "$ADDR" -db "$runtime_db" -ui "$runtime_ui" )
if [[ -n "$PULSE_GUARDIAN_INTERVAL" ]]; then
  server_args+=( -pulse-guardian-interval "$PULSE_GUARDIAN_INTERVAL" )
fi

# Start the server in the background so the recovery receiver can register its
# callback against an already-listening API, then wait for health before launch.
"$binary" "${server_args[@]}" &
server_pid=$!

# Wait until the dashboard is reachable (covers slow first builds / binds).
for _ in $(seq 1 50); do
  if curl --silent --output /dev/null --max-time 1 "http://$ADDR/health"; then
    break
  fi
  sleep 1
done

if [[ -n "$PULSE_GUARDIAN_INTERVAL" ]]; then
  guardian_binary="$STATE_DIR/bin/guardian"
  "$guardian_binary" -addr "$GUARDIAN_RECEIVER_ADDR" -state-dir "$STATE_DIR" -timekeeper-url "http://$ADDR" -agent "$GUARDIAN_RECEIVER_AGENT" &
  printf 'Pulse Guardian recovery receiver: http://%s/\n' "$GUARDIAN_RECEIVER_ADDR"
fi

wait "$server_pid"
