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
PROXY_ADDR="${TIMEKEEPER_PROXY_ADDR:-127.0.0.1:80}"
DB_PATH="$STATE_ROOT/timekeeper.db"
UI_PATH="$REPO/.timekeeper/web/index.html"
GUARDIAN_ADDR="${TIMEKEEPER_GUARDIAN_RECEIVER_ADDR:-127.0.0.1:1619}"
GUARDIAN_AGENT="${TIMEKEEPER_GUARDIAN_RECEIVER_AGENT:-xatia}"
GUARDIAN_INTERVAL="${TIMEKEEPER_PULSE_GUARDIAN_INTERVAL:-5m}"
SERVICE_NAME="TimeKeeper"

# Source the port-check library so tk_resolve_proxy_addr / tk_probe_loopback_port
# are available to win_install and linux_install. Safe to source multiple times.
SCRIPT_LIB="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib-port-check.sh"
if [[ -f "$SCRIPT_LIB" ]]; then
  # shellcheck disable=SC1090
  source "$SCRIPT_LIB"
fi

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
# find_nssm prints the resolved nssm.exe path on stdout and returns 0,
# or prints an actionable error to stderr and returns 1. The error
# names every location that was checked so the user can put nssm.exe
# somewhere reachable or set TIMEKEEPER_NSSM.
find_nssm() {
  local nssm=""
  local searched=()
  if [[ -n "${TIMEKEEPER_NSSM:-}" && -x "$TIMEKEEPER_NSSM" ]]; then
    nssm="$TIMEKEEPER_NSSM"
  fi
  if [[ -z "$nssm" ]] && [[ -x "$SERVICE_DIR/nssm/nssm.exe" ]]; then
    nssm="$SERVICE_DIR/nssm/nssm.exe"
  fi
  searched+=("$SERVICE_DIR/nssm/nssm.exe")
  if [[ -z "$nssm" ]] && [[ -x "/mnt/d/var/nssm/win64/nssm.exe" ]]; then
    nssm="/mnt/d/var/nssm/win64/nssm.exe"
  fi
  searched+=("/mnt/d/var/nssm/win64/nssm.exe")
  if [[ -z "$nssm" ]] && command -v nssm >/dev/null 2>&1; then
    nssm="$(command -v nssm)"
  fi
  searched+=("\$(command -v nssm) on PATH")
  if [[ -n "$nssm" ]]; then
    echo "$nssm"
    return 0
  fi
  cat >&2 <<EOF
[service:err] NSSM (the Non-Sucking Service Manager) is required to install
[service:err] TimeKeeper as a Windows service, but nssm.exe was not found.
[service:err]
[service:err] Searched:
$(printf '  - %s\n' "${searched[@]}" >&2)
[service:err]
[service:err] Fix (pick one):
[service:err]   1) Download nssm 2.24 from https://nssm.cc and copy the
[service:err]      win64/nssm.exe into $SERVICE_DIR/nssm/nssm.exe
[service:err]   2) Set TIMEKEEPER_NSSM=/path/to/nssm.exe before running
[service:err]      tk service install
[service:err]   3) Put nssm.exe on PATH
EOF
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
  if ! nssm="$(find_nssm)"; then
    # find_nssm already printed the actionable error.
    return 69
  fi
  verify_installed || return 1

  log "Using NSSM: $nssm"

  # Resolve the proxy address: env override wins, then INSTALLATION.env,
  # then probe the default port and prompt the user if it's in use.
  PROXY_ADDR="$(tk_resolve_proxy_addr "$PROXY_ADDR_DEFAULT")"
  if [[ -z "$PROXY_ADDR" ]]; then
    log "Friendly-URL proxy is disabled (TIMEKEEPER_PROXY_DISABLED=1)."
  else
    log "Using proxy address: $PROXY_ADDR"
  fi

  # Compose the desired service parameters. Keeping this in one
  # variable is what makes the idempotent check below possible:
  # compare it byte-for-byte against the live NSSM value.
  local desired_app_parameters="-addr $ADDR -db .timekeeper/timekeeper.db -ui .timekeeper/web/index.html -pulse-guardian-interval $GUARDIAN_INTERVAL -proxy-addr $PROXY_ADDR"
  local desired_app_directory="$REPO"
  local desired_display_name="TimeKeeper — Project Execution Memory"
  local desired_description="Local project execution memory for AI agents. Runs on loopback $ADDR."

  # Idempotent re-install: if the service is already installed and
  # every relevant parameter matches the desired value, skip the
  # remove + re-install cycle. This keeps the live process alive
  # across `tk service install` re-runs (the original behaviour
  # always stopped and reinstalled, which interrupted a running
  # process on every refresh).
  if "$nssm" status "$SERVICE_NAME" >/dev/null 2>&1; then
    local current_app_parameters current_app_directory current_display_name
    current_app_parameters="$("$nssm" get "$SERVICE_NAME" AppParameters 2>/dev/null || echo "")"
    current_app_directory="$("$nssm" get "$SERVICE_NAME" AppDirectory 2>/dev/null || echo "")"
    current_display_name="$("$nssm" get "$SERVICE_NAME" DisplayName 2>/dev/null || echo "")"
    if [[ "$current_app_parameters" == "$desired_app_parameters" ]] \
       && [[ "$current_app_directory" == "$desired_app_directory" ]] \
       && [[ "$current_display_name" == "$desired_display_name" ]]; then
      log "Service '$SERVICE_NAME' is already installed with the desired parameters; skipping reinstall."
      # Make sure it is running, in case it was stopped.
      "$nssm" status "$SERVICE_NAME" 2>/dev/null | grep -q RUNNING || "$nssm" start "$SERVICE_NAME"
      log "Service is up. Logs: $LOG_DIR/service.log"
      return 0
    fi
    log "Existing service has stale parameters; updating in place."
    "$nssm" stop "$SERVICE_NAME" >/dev/null 2>&1 || true
    "$nssm" remove "$SERVICE_NAME" confirm >/dev/null 2>&1 || true
    sleep 1
  fi

  # Install the service
  "$nssm" install "$SERVICE_NAME" "$BIN"
  "$nssm" set "$SERVICE_NAME" AppDirectory "$desired_app_directory"
  "$nssm" set "$SERVICE_NAME" AppParameters "$desired_app_parameters"
  "$nssm" set "$SERVICE_NAME" DisplayName "$desired_display_name"
  "$nssm" set "$SERVICE_NAME" Description "$desired_description"
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
  if ! nssm="$(find_nssm)"; then
    return 69
  fi

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
  if ! nssm="$(find_nssm)"; then return 69; fi
  "$nssm" start "$SERVICE_NAME"
  log "Service started"
}

win_stop() {
  local nssm
  if ! nssm="$(find_nssm)"; then return 69; fi
  "$nssm" stop "$SERVICE_NAME"
  log "Service stopped"
}

win_restart() {
  local nssm
  if ! nssm="$(find_nssm)"; then return 69; fi
  "$nssm" restart "$SERVICE_NAME"
  log "Service restarted"
}

win_status() {
  local nssm
  if ! nssm="$(find_nssm)"; then return 69; fi
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
ExecStart=$BIN -addr $ADDR -db $DB_PATH -ui $UI_PATH -pulse-guardian-interval $GUARDIAN_INTERVAL -proxy-addr $PROXY_ADDR
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

  # Resolve the proxy address: env override wins, then INSTALLATION.env,
  # then probe the default port and prompt the user if it's in use.
  PROXY_ADDR="$(tk_resolve_proxy_addr "$PROXY_ADDR_DEFAULT")"
  if [[ -z "$PROXY_ADDR" ]]; then
    log "Friendly-URL proxy is disabled (TIMEKEEPER_PROXY_DISABLED=1)."
  else
    log "Using proxy address: $PROXY_ADDR"
  fi

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
