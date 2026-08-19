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
Usage: ./install.sh

Build Time Keeper from this verified checkout into .timekeeper/app.
The installed app, its launchers, and its local SQLite state stay with this clone.

The installer does not download code, change PATH, request privileges, create a
service, or start a server. Stop a running Time Keeper process before refreshing
an existing installation.
USAGE
}

fail() {
  printf 'Time Keeper install: %s\n' "$1" >&2
  exit "${2:-64}"
}

if (($#)); then
  if [[ "$1" == '--help' || "$1" == '-h' ]]; then
    usage
    exit 0
  fi
  usage >&2
  fail 'install.sh takes no arguments; run it from the Time Keeper checkout.'
fi

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

mkdir -p "$INSTALL_ROOT/app" "$INSTALL_ROOT/bin" "$INSTALL_ROOT/state"
chmod 700 "$INSTALL_ROOT" "$INSTALL_ROOT/bin" "$INSTALL_ROOT/state"
git -C "$SOURCE" archive --format=tar "$COMMIT" | tar -xf - -C "$INSTALL_ROOT/app"
[[ -f "$INSTALL_ROOT/app/go.mod" && -f "$INSTALL_ROOT/app/web/timekeeper.html" ]] || fail 'verified source archive is missing required application files.'

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
UI_PATH="\${TIMEKEEPER_UI:-\$ROOT/app/web/timekeeper.html}"
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
