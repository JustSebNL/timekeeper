#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
# TIMEKEEPER_CROSS_PLATFORM_SERVICE_MANAGER
# Manages TimeKeeper as an OS service (Windows NSSM / Linux systemd).
# All artifacts live under the repo's .timekeeper/ directory.
set -Eeuo pipefail

# Resolve repo root: the script lives at .timekeeper/app/scripts/service/,
# so we walk up until we find the repo root (where install.sh / go.mod live).
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$SCRIPT_DIR"
while [[ "$REPO" != "/" ]]; do
  if [[ -f "$REPO/install.sh" && -f "$REPO/go.mod" ]]; then
    break
  fi
  REPO="$(dirname "$REPO")"
done
if [[ "$REPO" == "/" ]]; then
  echo "[service:err] could not find repo root (no install.sh + go.mod marker)" >&2
  exit 1
fi
STATE_ROOT="$REPO/.timekeeper"
SERVICE_DIR="$STATE_ROOT/service"
LOG_DIR="$STATE_ROOT/log"
BIN="$STATE_ROOT/app/bin/timekeeper"
CLI_BIN="$STATE_ROOT/app/bin/tk"
GUARDIAN_BIN="$STATE_ROOT/app/bin/guardian"
ADDR="${TIMEKEEPER_ADDR:-127.0.0.1:1618}"
DB_PATH="$STATE_ROOT/timekeeper.db"
UI_PATH="$REPO/web/timekeeper.html"
GUARDIAN_ADDR="${TIMEKEEPER_GUARDIAN_RECEIVER_ADDR:-127.0.0.1:1619}"
GUARDIAN_AGENT="${TIMEKEEPER_GUARDIAN_RECEIVER_AGENT:-xatia}"
GUARDIAN_INTERVAL="${TIMEKEEPER_PULSE_GUARDIAN_INTERVAL:-5m}"
SERVICE_NAME="TimeKeeper"

mkdir -p "$SERVICE_DIR" "$LOG_DIR"

usage() {
  cat <<USAGE
Usage: $(basename "$0") <command>

Commands:
  install     Install TimeKeeper as an OS service
  uninstall   Remove the OS service
  start       Start the service
  stop        Stop the service
  restart     Restart the service
  status      Show service status
  logs        Show recent service log tail

Windows: uses NSSM (auto-detected or set TIMEKEEPER_NSSM)
Linux:   uses systemd (user scope, linger-enabled)

All service artifacts are stored under: $STATE_ROOT/service/
USAGE
}

log() { printf '[service] %s\n' "$1"; }
err() { printf '[service:err] %s\n' "$1" >&2; }

# ─── helpers ────────────────────────────────────────────────────────────────
find_nssm() {
  local nssm=""
  if [[ -n "${TIMEKEEPER_NSSM:-}" && -x "$TIMEKEEPER_NSSM" ]]; then
    nssm="$TIMEKEEPER_NSSM"
  elif [[ -x "$SERVICE_DIR/nssm/nssm.exe" ]]; then
    nssm="$SERVICE_DIR/nssm/nssm.exe"
  elif [[ -x "/mnt/d/var/nssm/win64/nssm.exe" ]]; then
    nssm="/mnt/d/var/nssm/win64/nssm.exe"
  elif command -v nssm >/dev/null 2>&1; then
    nssm="$(command -v nssm)"
  fi
  if [[ -n "$nssm" ]]; then
    echo "$nssm"
    return 0
  fi
  return 1
}

verify_installed() {
  if [[ ! -x "$BIN" ]]; then
    err "TimeKeeper binary not found at $BIN — run install.sh first"
    return 1
  fi
}

# ─── Windows (NSSM) ────────────────────────────────────────────────────────
win_install() {
  local nssm
  nssm="$(find_nssm)" || {
    err "NSSM not found. Install it or set TIMEKEEPER_NSSM to nssm.exe"
    return 69
  }
  verify_installed || return 1

  log "Using NSSM: $nssm"

  # If service already exists, remove it first
  if "$nssm" status "$SERVICE_NAME" >/dev/null 2>&1; then
    log "Existing service found — removing first"
    "$nssm" stop "$SERVICE_NAME" >/dev/null 2>&1 || true
    "$nssm" remove "$SERVICE_NAME" confirm >/dev/null 2>&1 || true
    sleep 1
  fi

  # Install the service
  "$nssm" install "$SERVICE_NAME" "$BIN"
  "$nssm" set "$SERVICE_NAME" AppDirectory "$REPO"
  "$nssm" set "$SERVICE_NAME" AppParameters "-addr $ADDR -db .timekeeper/timekeeper.db -ui web/timekeeper.html -pulse-guardian-interval $GUARDIAN_INTERVAL"
  "$nssm" set "$SERVICE_NAME" DisplayName "TimeKeeper — Project Execution Memory"
  "$nssm" set "$SERVICE_NAME" Description "Local project execution memory for AI agents. Runs on loopback $ADDR."
  "$nssm" set "$SERVICE_NAME" Start SERVICE_DELAYED_AUTO_START
  "$nssm" set "$SERVICE_NAME" AppStdout "$LOG_DIR/service.log"
  "$nssm" set "$SERVICE_NAME" AppStderr "$LOG_DIR/service.error.log"
  "$nssm" set "$SERVICE_NAME" AppStdoutCreationDisposition 4
  "$nssm" set "$SERVICE_NAME" AppStderrCreationDisposition 4
  "$nssm" set "$SERVICE_NAME" AppRotateFiles 1
  "$nssm" set "$SERVICE_NAME" AppRotateBytes 10485760
  "$nssm" set "$SERVICE_NAME" AppExit Default Restart
  "$nssm" set "$SERVICE_NAME" AppRestartDelay 5000
  "$nssm" set "$SERVICE_NAME" AppThrottle 30000

  log "Service installed with delayed auto-start"
  log "Starting service..."
  "$nssm" start "$SERVICE_NAME"
  log "TimeKeeper service '$SERVICE_NAME' installed and started"
  log "Logs: $LOG_DIR/service.log"
}

win_uninstall() {
  local nssm
  nssm="$(find_nssm)" || { err "NSSM not found"; return 69; }

  if ! "$nssm" status "$SERVICE_NAME" >/dev/null 2>&1; then
    log "Service '$SERVICE_NAME' is not installed"
    return 0
  fi

  "$nssm" stop "$SERVICE_NAME" >/dev/null 2>&1 || true
  "$nssm" remove "$SERVICE_NAME" confirm
  log "Service '$SERVICE_NAME' removed"
}

win_start() {
  local nssm
  nssm="$(find_nssm)" || { err "NSSM not found"; return 69; }
  "$nssm" start "$SERVICE_NAME"
  log "Service started"
}

win_stop() {
  local nssm
  nssm="$(find_nssm)" || { err "NSSM not found"; return 69; }
  "$nssm" stop "$SERVICE_NAME"
  log "Service stopped"
}

win_restart() {
  local nssm
  nssm="$(find_nssm)" || { err "NSSM not found"; return 69; }
  "$nssm" restart "$SERVICE_NAME"
  log "Service restarted"
}

win_status() {
  local nssm
  nssm="$(find_nssm)" || { err "NSSM not found"; return 69; }
  if "$nssm" status "$SERVICE_NAME" >/dev/null 2>&1; then
    log "Service '$SERVICE_NAME' is installed"
    "$nssm" status "$SERVICE_NAME"
  else
    log "Service '$SERVICE_NAME' is NOT installed"
    return 1
  fi
}

# ─── Linux (systemd) ────────────────────────────────────────────────────────
linux_systemd_unit() {
  local unit_dir="$HOME/.config/systemd/user"
  mkdir -p "$unit_dir"
  cat > "$unit_dir/timekeeper.service" <<EOF
[Unit]
Description=TimeKeeper — Project Execution Memory
After=network.target
StartLimitIntervalSec=300
StartLimitBurst=5

[Service]
Type=simple
WorkingDirectory=$REPO
ExecStart=$BIN -addr $ADDR -db $DB_PATH -ui $UI_PATH -pulse-guardian-interval $GUARDIAN_INTERVAL
ExecStop=/bin/kill -SIGTERM \$MAINPID
Restart=on-failure
RestartSec=5
StandardOutput=append:$LOG_DIR/service.log
StandardError=append:$LOG_DIR/service.error.log
Environment="TIMEKEEPER_URL=http://$ADDR"

[Install]
WantedBy=default.target
EOF
  echo "$unit_dir/timekeeper.service"
}

linux_install() {
  verify_installed || return 1

  local unit
  unit="$(linux_systemd_unit)"
  log "Systemd unit: $unit"

  # Enable linger so the service runs without an active login session
  if command -v loginctl >/dev/null 2>&1; then
    loginctl enable-linger "$(whoami)" 2>/dev/null || true
  fi

  systemctl --user daemon-reload
  systemctl --user enable timekeeper.service
  systemctl --user start timekeeper.service
  log "TimeKeeper systemd service installed and started"
  log "Logs: $LOG_DIR/service.log"
}

linux_uninstall() {
  if systemctl --user list-unit-files timekeeper.service >/dev/null 2>&1; then
    systemctl --user stop timekeeper.service 2>/dev/null || true
    systemctl --user disable timekeeper.service 2>/dev/null || true
    rm -f "$HOME/.config/systemd/user/timekeeper.service"
    systemctl --user daemon-reload
    log "Systemd service removed"
  else
    log "Systemd service 'timekeeper' is not installed"
  fi
}

linux_start() {
  systemctl --user start timekeeper.service
  log "Service started"
}

linux_stop() {
  systemctl --user stop timekeeper.service
  log "Service stopped"
}

linux_restart() {
  systemctl --user restart timekeeper.service
  log "Service restarted"
}

linux_status() {
  systemctl --user status timekeeper.service --no-pager
}

# ─── logs ────────────────────────────────────────────────────────────────────
show_logs() {
  local log_file="$LOG_DIR/service.log"
  if [[ -f "$log_file" ]]; then
    tail -50 "$log_file"
  else
    log "No log file at $log_file"
  fi
}

# ─── dispatch ───────────────────────────────────────────────────────────────
os="$(uname -s)"
case "${os:-}" in
  Linux*)  platform="linux" ;;
  CYGWIN*|MINGW*|MSYS*) platform="windows" ;;
  *)       err "unsupported OS: $os"; exit 64 ;;
esac

cmd="${1:-}"
shift || true

case "$cmd" in
  install)
    [[ "$platform" == "windows" ]] && win_install || linux_install ;;
  uninstall|remove)
    [[ "$platform" == "windows" ]] && win_uninstall || linux_uninstall ;;
  start)
    [[ "$platform" == "windows" ]] && win_start || linux_start ;;
  stop)
    [[ "$platform" == "windows" ]] && win_stop || linux_stop ;;
  restart)
    [[ "$platform" == "windows" ]] && win_restart || linux_restart ;;
  status)
    [[ "$platform" == "windows" ]] && win_status || linux_status ;;
  logs)
    show_logs ;;
  *)
    usage >&2; exit 2 ;;
esac
