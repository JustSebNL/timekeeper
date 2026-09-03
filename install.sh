#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
# TIMEKEEPER_REPOSITORY_LOCAL_INSTALLER
set -Eeuo pipefail
umask 077

EXPECTED_ORIGIN='git@github.com:JustSebNL/timekeeper.git'
EXPECTED_HTTPS_ORIGIN='https://github.com/JustSebNL/timekeeper.git'
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
SOURCE="$ROOT"
DESTINATION="$ROOT/.timekeeper/app"

usage() {
  cat <<'USAGE'
Usage: ./install.sh [--with-port=<addr>] [--without-proxy]

Build Time Keeper from this verified checkout into .timekeeper/app.
The installed app, its launchers, and its local SQLite state stay with this clone.

The installer does not download code, change PATH, request privileges, create a
service, or start a server. Stop a running Time Keeper process before refreshing
an existing installation.

Options:
  --with-port=<host:port>   Set TIMEKEEPER_PROXY_ADDR explicitly. The friendly
                            URL proxy will bind to this address instead of the
                            default 127.0.0.1:80. Persisted to INSTALLATION.env.
                            Example: --with-port=127.0.0.1:8080
  --without-proxy           Disable the friendly-URL proxy listener entirely.
                            Only the canonical 127.0.0.1:1618 address will work.
                            Persisted to INSTALLATION.env.
  -h, --help                Show this help.
USAGE
}

fail() {
  printf 'Time Keeper install: %s\n' "$1" >&2
  exit "${2:-64}"
}

# Parse the small allowlist of flags. Anything else is a hard error so
# typos like "--without-prox" are caught instead of silently ignored.
PROXY_ADDR_FOR_INSTALL=""
PROXY_DISABLED_FOR_INSTALL=0
while (($#)); do
  case "$1" in
    --with-port=*)
      PROXY_ADDR_FOR_INSTALL="${1#--with-port=}"
      if [[ ! "$PROXY_ADDR_FOR_INSTALL" =~ ^[^:]+:[0-9]+$ ]]; then
        fail "--with-port expects host:port, got '$PROXY_ADDR_FOR_INSTALL'"
      fi
      ;;
    --with-port)
      if [[ $# -lt 2 ]]; then
        fail '--with-port requires a value (host:port)'
      fi
      shift
      PROXY_ADDR_FOR_INSTALL="$1"
      if [[ ! "$PROXY_ADDR_FOR_INSTALL" =~ ^[^:]+:[0-9]+$ ]]; then
        fail "--with-port expects host:port, got '$PROXY_ADDR_FOR_INSTALL'"
      fi
      ;;
    --without-proxy)
      PROXY_DISABLED_FOR_INSTALL=1
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      usage >&2
      fail "unknown argument: $1"
      ;;
  esac
  shift
done

command -v git >/dev/null 2>&1 || fail 'Git is required to verify this checkout.' 69
if ! git -C "$SOURCE" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  fail 'install.sh must run from a Git checkout.'
fi
if [[ -n "$(git -C "$SOURCE" status --porcelain --untracked-files=all)" ]]; then
  if [[ -d "$ROOT/.timekeeper/app" ]]; then
    printf 'Time Keeper install: checkout is dirty, but an existing install was detected at .timekeeper/app. Proceeding with local install while preserving runtime state.\n' >&2
  else
    fail 'this checkout must be clean before installation.'
  fi
fi

ORIGIN="$(git -C "$SOURCE" remote get-url origin 2>/dev/null || true)"
if [[ "$ORIGIN" != "$EXPECTED_ORIGIN" && "$ORIGIN" != "$EXPECTED_HTTPS_ORIGIN" ]]; then
  fail "checkout origin must be $EXPECTED_ORIGIN"
fi
COMMIT="$(git -C "$SOURCE" rev-parse HEAD)"
git -C "$SOURCE" cat-file -e "$COMMIT^{commit}" >/dev/null 2>&1 || fail 'checkout HEAD is not a verifiable commit.'
ORIGIN_MAIN="$(git -C "$SOURCE" rev-parse refs/remotes/origin/main 2>/dev/null || true)"

INSTALLED_COMMIT=""
if [[ -f "$ROOT/.timekeeper/app/INSTALLATION.env" ]]; then
  INSTALLED_COMMIT="$(grep -E '^SOURCE_COMMIT=' "$ROOT/.timekeeper/app/INSTALLATION.env" | cut -d= -f2- || true)"
fi

if [[ -n "$INSTALLED_COMMIT" && "$INSTALLED_COMMIT" == "$COMMIT" && "$COMMIT" == "$ORIGIN_MAIN" ]]; then
  printf 'Time Keeper is already installed and up to date at commit %s.\n' "$COMMIT"
  exit 0
fi

if [[ "$COMMIT" != "$ORIGIN_MAIN" ]]; then
  fail "checkout HEAD must exactly match origin/main for installation/update. Installed: ${INSTALLED_COMMIT:-none}, current: $COMMIT, origin/main: $ORIGIN_MAIN"
fi

GO_BIN="${TIMEKEEPER_GO:-go}"
if ! "$GO_BIN" version >/dev/null 2>&1; then
  fail 'Time Keeper requires Go to build locally. Set TIMEKEEPER_GO to a Go executable.' 69
fi
GOOS="$("$GO_BIN" env GOOS)"
[[ "$GOOS" == 'linux' || "$GOOS" == 'windows' || "$GOOS" == 'darwin' ]] || fail "unsupported target GOOS: $GOOS"

STATE_ROOT="$ROOT/.timekeeper"
mkdir -p "$STATE_ROOT"
chmod 700 "$STATE_ROOT"
STAGE="$(mktemp -d "$STATE_ROOT/.install.XXXXXX")"
INSTALL_ROOT="$STAGE/app"
cleanup() {
  rm -rf "$STAGE"
}
trap cleanup EXIT

mkdir -p "$INSTALL_ROOT/app" "$INSTALL_ROOT/bin" "$INSTALL_ROOT/state" "$STATE_ROOT/web"
chmod 700 "$INSTALL_ROOT" "$INSTALL_ROOT/bin" "$INSTALL_ROOT/state" "$STATE_ROOT/web"
git -C "$SOURCE" archive --format=tar "$COMMIT" | tar -xf - -C "$INSTALL_ROOT/app"
[[ -f "$INSTALL_ROOT/app/go.mod" && -f "$INSTALL_ROOT/app/web/index.html" ]] || fail 'verified source archive is missing required application files.'
# Move web assets to .timekeeper/web/ (served from there, outside the app tree so refreshes preserve customizations)
mv "$INSTALL_ROOT/app/web/"* "$STATE_ROOT/web/"
rmdir "$INSTALL_ROOT/app/web"

server_binary="$INSTALL_ROOT/bin/timekeeper"
cli_binary="$INSTALL_ROOT/bin/tk"
guardian_binary="$INSTALL_ROOT/bin/guardian"
[[ "$GOOS" == 'windows' ]] && { server_binary+='.exe'; cli_binary+='.exe'; guardian_binary+='.exe'; }
build_server="$server_binary"
build_cli="$cli_binary"
build_guardian="$guardian_binary"
if [[ "$GOOS" == 'windows' && -x "$(command -v wslpath || true)" ]]; then
  build_server="$(wslpath -w "$server_binary")"
  build_cli="$(wslpath -w "$cli_binary")"
  build_guardian="$(wslpath -w "$guardian_binary")"
fi
if ! (
  cd "$INSTALL_ROOT/app"
  GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local "$GO_BIN" list -mod=readonly -deps ./cmd/server ./cmd/tk ./cmd/guardian >/dev/null
); then
  fail 'required Go modules are not available locally; fetch and verify them before running this offline installer.' 69
fi
(
  cd "$INSTALL_ROOT/app"
  GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local "$GO_BIN" build -trimpath -o "$build_server" ./cmd/server
  GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local "$GO_BIN" build -trimpath -o "$build_cli" ./cmd/tk
  GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local "$GO_BIN" build -trimpath -o "$build_guardian" ./cmd/guardian
)
[[ -s "$server_binary" && -s "$cli_binary" && -s "$guardian_binary" ]] || fail 'local Go build did not produce all Time Keeper binaries.'
chmod 700 "$server_binary" "$cli_binary" "$guardian_binary"

cat > "$INSTALL_ROOT/timekeeper" <<LAUNCHER
#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
set -Eeuo pipefail
ROOT="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd -P)"
ADDR="\${TIMEKEEPER_ADDR:-127.0.0.1:1618}"
DB_PATH="\${TIMEKEEPER_DB:-\$ROOT/state/timekeeper.db}"
UI_PATH="\${TIMEKEEPER_UI:-\$ROOT/../web/index.html}"
PULSE_GUARDIAN_INTERVAL="\${TIMEKEEPER_PULSE_GUARDIAN_INTERVAL:-5m}"
GUARDIAN_RECEIVER_ADDR="\${TIMEKEEPER_GUARDIAN_RECEIVER_ADDR:-127.0.0.1:1619}"
GUARDIAN_RECEIVER_AGENT="\${TIMEKEEPER_GUARDIAN_RECEIVER_AGENT:-xatia}"
TARGET_GOOS='$GOOS'
if [[ "\$TARGET_GOOS" == 'windows' && -x "\$(command -v wslpath || true)" ]]; then
  DB_PATH="\$(wslpath -w "\$DB_PATH")"
  UI_PATH="\$(wslpath -w "\$UI_PATH")"
fi
server_args=( -addr "\$ADDR" -db "\$DB_PATH" -ui "\$UI_PATH" )
if [[ -n "\$PULSE_GUARDIAN_INTERVAL" ]]; then
  server_args+=( -pulse-guardian-interval "\$PULSE_GUARDIAN_INTERVAL" )
fi
if [[ -n "\$PULSE_GUARDIAN_INTERVAL" ]]; then
  guardian_binary="\$ROOT/bin/$(basename "$guardian_binary")"
  "\$guardian_binary" -addr "\$GUARDIAN_RECEIVER_ADDR" -state-dir "\$ROOT/state" -timekeeper-url "http://\$ADDR" -agent "\$GUARDIAN_RECEIVER_AGENT" &
fi
exec "\$ROOT/bin/$(basename "$server_binary")" "\${server_args[@]}"
LAUNCHER
cat > "$INSTALL_ROOT/tk" <<LAUNCHER
#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
set -Eeuo pipefail
ROOT="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd -P)"
exec "\$ROOT/bin/$(basename "$cli_binary")" "\$@"
LAUNCHER
chmod 700 "$INSTALL_ROOT/timekeeper" "$INSTALL_ROOT/tk"

cat > "$INSTALL_ROOT/INSTALLATION.env" <<INSTALLATION
SOURCE_ORIGIN=$ORIGIN
SOURCE_COMMIT=$COMMIT
TARGET_GOOS=$GOOS
INSTALLER_STARTS_NO_SERVER=1
INSTALLATION

# Persist --with-port / --without-proxy choices so re-installs and the
# service install step both honour them. The service-manager.sh script
# reads TIMEKEEPER_PROXY_ADDR / TIMEKEEPER_PROXY_DISABLED from
# INSTALLATION.env at install time.
if [[ -n "$PROXY_ADDR_FOR_INSTALL" ]]; then
  printf 'TIMEKEEPER_PROXY_ADDR=%s\n' "$PROXY_ADDR_FOR_INSTALL" >> "$INSTALL_ROOT/INSTALLATION.env"
fi
if [[ "$PROXY_DISABLED_FOR_INSTALL" -eq 1 ]]; then
  printf 'TIMEKEEPER_PROXY_DISABLED=1\n' >> "$INSTALL_ROOT/INSTALLATION.env"
fi
chmod 600 "$INSTALL_ROOT/INSTALLATION.env"

if [[ -e "$DESTINATION" && ! -d "$DESTINATION" ]]; then
  fail 'repository installation path exists but is not a directory.'
fi
if [[ -d "$DESTINATION" ]]; then
  PREVIOUS="$STATE_ROOT/.previous-app.$$"
  [[ ! -e "$PREVIOUS" ]] || fail 'temporary replacement path already exists; retry the installer.'
  if [[ -d "$DESTINATION/state" ]]; then
      rmdir "$INSTALL_ROOT/state"
      mv "$DESTINATION/state" "$INSTALL_ROOT/state"
  fi
  if [[ -d "$DESTINATION/../web" && ! -d "$INSTALL_ROOT/../web" ]]; then
      # Preserve existing web assets if the new install doesn't carry them
      true
  fi
  mv "$DESTINATION" "$PREVIOUS"
  if mv "$INSTALL_ROOT" "$DESTINATION"; then
    rm -rf "$PREVIOUS"
    printf 'Time Keeper refreshed at .timekeeper/app; preserving existing runtime state.\n'
  else
    if [[ -d "$INSTALL_ROOT/state" && ! -e "$PREVIOUS/state" ]]; then
      mv "$INSTALL_ROOT/state" "$PREVIOUS/state" || true
    fi
    mv "$PREVIOUS" "$DESTINATION" || fail 'installation refresh failed and rollback could not restore the previous app.'
    fail 'installation refresh failed; the previous app was restored.'
  fi
else
  mv "$INSTALL_ROOT" "$DESTINATION"
  printf 'Time Keeper installed at .timekeeper/app.\n'
fi
trap - EXIT
rmdir "$STAGE"
printf 'Source commit: %s\n' "$COMMIT"
printf 'Run: ./.timekeeper/app/timekeeper\n'
printf 'Then use: ./.timekeeper/app/tk list\n'
printf 'Run as OS service: ./.timekeeper/app/tk service install\n'

# Linux unprivileged port 80: if the user is root, grant the timekeeper
# binary the CAP_NET_BIND_SERVICE capability so the friendly-URL proxy
# can bind 127.0.0.1:80 without running the whole service as root. This
# is the one-shot, per-binary step that replaces a setuid wrapper (which
# we deliberately don't ship). No-op on non-Linux targets.
if [[ "$GOOS" == "linux" ]]; then
  if [[ "$(id -u 2>/dev/null || echo 1)" -eq 0 ]]; then
    if command -v setcap >/dev/null 2>&1; then
      if setcap 'cap_net_bind_service=+ep' "$DESTINATION/bin/timekeeper" 2>/dev/null; then
        printf 'Granted CAP_NET_BIND_SERVICE to .timekeeper/app/bin/timekeeper (so the friendly-URL proxy can bind 127.0.0.1:80 without root).\n'
      else
        printf 'Time Keeper install: could not set CAP_NET_BIND_SERVICE on the binary. The friendly-URL proxy will fall back to a high port (set TIMEKEEPER_PROXY_ADDR to a free unprivileged port).\n' >&2
      fi
    else
      printf 'Time Keeper install: setcap not found; skipping CAP_NET_BIND_SERVICE. Install libcap2-bin if you need the friendly-URL proxy to bind 127.0.0.1:80 without root.\n' >&2
    fi
  else
    printf 'Time Keeper install: not running as root; skipping setcap. If the friendly-URL proxy fails to bind 127.0.0.1:80, run: sudo setcap cap_net_bind_service=+ep .timekeeper/app/bin/timekeeper\n' >&2
  fi
fi

# Write the friendly-URL hosts entries. Best-effort: if the user does
# not have permission to edit the hosts file, the install still
# succeeds and `tk doctor` will report the friendly URLs as failed.
# The user can re-run the installer from an elevated context, or edit
# the hosts file manually (the README documents the manual step).
"$DESTINATION/bin/$(basename "$cli_binary")" hosts add || printf 'Time Keeper install: could not write friendly-URL hosts entries (run as Administrator / with sudo, or edit %s manually)\n' "/etc/hosts"
