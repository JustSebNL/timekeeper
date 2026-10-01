#!/usr/bin/env bash
# TimeKeeper Linux service wrapper (systemd user-scope)
# Installs TimeKeeper as a systemd user service, then cleans up the legacy 30-minute runner.
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$SCRIPT_DIR/.."   # .timekeeper/scripts/service -> .timekeeper/ -> repo root candidate
# Walk up to the real repo root (where install.sh + go.mod live)
while [[ "$REPO" != "/" ]]; do
  if [[ -f "$REPO/install.sh" && -f "$REPO/go.mod" ]]; then break; fi
  REPO="$(dirname "$REPO")"
done
if [[ "$REPO" == "/" ]]; then
  echo "[service] could not find repo root (install.sh + go.mod) from $SCRIPT_DIR" >&2; exit 1
fi
REPO="$REPO"
SERVICE_DIR="$REPO/.timekeeper/service"
LOG_DIR="$REPO/.timekeeper/log"
BIN="$REPO/.timekeeper/app/bin/timekeeper"
ADDR="${TIMEKEEPER_ADDR:-127.0.0.1:1618}"
PROXY_ADDR="${TIMEKEEPER_PROXY_ADDR:-127.0.0.1:80}"
DB_PATH="$REPO/.timekeeper/timekeeper.db"
UI_PATH="$REPO/.timekeeper/web/index.html"
GUARDIAN_INTERVAL="${TIMEKEEPER_PULSE_GUARDIAN_INTERVAL:-5m}"
SERVICE_NAME="timekeeper"

mkdir -p "$SERVICE_DIR" "$LOG_DIR"
UNIT_FILE="$HOME/.config/systemd/user/$SERVICE_NAME.service"

cmd="${1:-}"
if [[ -z "$cmd" ]]; then
  echo "Usage: service-manager.sh <install|uninstall|start|stop|restart|status|logs|cleanup30min>" >&2
  exit 64
fi

show_logs() {
  [[ -f "$LOG_DIR/service.log" ]] && tail -50 "$LOG_DIR/service.log" || echo "[service] no log at $LOG_DIR/service.log"
}
cleanup30min() {
  echo "[service] legacy 30-minute runner disabled (no-op on Linux; windows handled in .bat)"
}

case "$cmd" in
 install)
   mkdir -p "$HOME/.config/systemd/user"
   cat > "$UNIT_FILE" <<EOF
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
   echo "[service] Systemd unit: $UNIT_FILE"
   command -v loginctl >/dev/null 2>&1 && loginctl enable-linger "$(whoami)" 2>/dev/null || true
   systemctl --user daemon-reload
   systemctl --user enable "$SERVICE_NAME"
   systemctl --user start "$SERVICE_NAME"
   echo "[service] installed+started $SERVICE_NAME; logs: $LOG_DIR/service.log"
   ;;
 uninstall|remove)
   systemctl --user stop "$SERVICE_NAME" 2>/dev/null || true
   systemctl --user disable "$SERVICE_NAME" 2>/dev/null || true
   rm -f "$UNIT_FILE"
   systemctl --user daemon-reload
   echo "[service] uninstalled $SERVICE_NAME"
   ;;
 start) systemctl --user start "$SERVICE_NAME" ;;
 stop)  systemctl --user stop "$SERVICE_NAME" ;;
 restart) systemctl --user restart "$SERVICE_NAME" ;;
 status) systemctl --user status "$SERVICE_NAME" --no-pager ;;
 logs)  show_logs ;;
 cleanup30min) cleanup30min ;;
 *) echo "Unknown command: $cmd" >&2; exit 64 ;;
esac
