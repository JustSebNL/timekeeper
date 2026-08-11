#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
# TIMEKEEPER_PINNED_LOCAL_BOOTSTRAP
set -Eeuo pipefail
umask 077

EXPECTED_ORIGIN='git@github.com:JustSebNL/timekeeper.git'
EXPECTED_HTTPS_ORIGIN='https://github.com/JustSebNL/timekeeper.git'
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
SOURCE="$ROOT"
DESTINATION=''

usage() {
  cat <<'USAGE'
Usage: ./install.sh [--source <clean-timekeeper-checkout>] [--destination <new-directory>]

Build and install Time Keeper from a clean, committed checkout of the authoritative
JustSebNL/timekeeper repository. The installed tree records the exact source commit.

Defaults:
  source:       directory containing this installer
  destination:  ~/.local/share/timekeeper on Linux/WSL
                %LOCALAPPDATA%\TimeKeeper on native Windows shells

The installer never downloads code, modifies PATH, elevates privileges, creates a
service, or starts Time Keeper. It refuses an existing destination and any source
checkout that is dirty, detached from origin/main, or not the expected repository.
USAGE
}

fail() {
  printf 'Time Keeper install: %s\n' "$1" >&2
  exit "${2:-64}"
}

windows_path_to_shell_path() {
  local path="$1"
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -u "$path"
  else
    printf '%s' "$path"
  fi
}

default_destination() {
  if [[ "${OS:-}" == 'Windows_NT' ]]; then
    [[ -n "${LOCALAPPDATA:-}" ]] || fail 'LOCALAPPDATA is required for a native Windows installation.'
    printf '%s/TimeKeeper' "$(windows_path_to_shell_path "$LOCALAPPDATA")"
    return
  fi
  printf '%s/.local/share/timekeeper' "$HOME"
}

while (($#)); do
  case "$1" in
    --source)
      (($# >= 2)) || fail '--source requires a directory.'
      SOURCE="$2"
      shift 2
      ;;
    --destination)
      (($# >= 2)) || fail '--destination requires a new directory path.'
      DESTINATION="$2"
      shift 2
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
done

[[ -d "$SOURCE" ]] || fail "source checkout does not exist: $SOURCE"
SOURCE="$(cd "$SOURCE" && pwd -P)"
[[ -n "$DESTINATION" ]] || DESTINATION="$(default_destination)"
[[ "$DESTINATION" != '/' && "$DESTINATION" != '.' ]] || fail 'destination must name a dedicated Time Keeper directory.'

if [[ -e "$DESTINATION" ]]; then
  fail "refusing non-empty destination: $DESTINATION already exists. Choose a new destination; installation never merges or overwrites."
fi

command -v git >/dev/null 2>&1 || fail 'Git is required to verify the source checkout.' 69
if ! git -C "$SOURCE" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  fail "source is not a Git checkout: $SOURCE"
fi
if [[ -n "$(git -C "$SOURCE" status --porcelain --untracked-files=all)" ]]; then
  fail 'source checkout must be clean before installation.'
fi

ORIGIN="$(git -C "$SOURCE" remote get-url origin 2>/dev/null || true)"
if [[ "$ORIGIN" != "$EXPECTED_ORIGIN" && "$ORIGIN" != "$EXPECTED_HTTPS_ORIGIN" ]]; then
  fail "source origin must be $EXPECTED_ORIGIN"
fi
COMMIT="$(git -C "$SOURCE" rev-parse HEAD)"
git -C "$SOURCE" cat-file -e "$COMMIT^{commit}" >/dev/null 2>&1 || fail 'source HEAD is not a verifiable commit.'
ORIGIN_MAIN="$(git -C "$SOURCE" rev-parse refs/remotes/origin/main 2>/dev/null || true)"
[[ "$COMMIT" == "$ORIGIN_MAIN" ]] || fail 'source HEAD must exactly match the locally verified origin/main revision.'

GO_BIN="${TIMEKEEPER_GO:-go}"
if ! "$GO_BIN" version >/dev/null 2>&1; then
  fail 'Time Keeper requires Go to build locally. Set TIMEKEEPER_GO to a Go executable.' 69
fi
GOOS="$("$GO_BIN" env GOOS)"
[[ "$GOOS" == 'linux' || "$GOOS" == 'windows' || "$GOOS" == 'darwin' ]] || fail "unsupported target GOOS: $GOOS"

DESTINATION_PARENT="$(dirname "$DESTINATION")"
mkdir -p "$DESTINATION_PARENT"
STAGE="$(mktemp -d "$DESTINATION_PARENT/.timekeeper-install.XXXXXX")"
INSTALL_ROOT="$STAGE/timekeeper"
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
[[ "$GOOS" == 'windows' ]] && { server_binary+='.exe'; cli_binary+='.exe'; }
build_server="$server_binary"
build_cli="$cli_binary"
if [[ "$GOOS" == 'windows' && -x "$(command -v wslpath || true)" ]]; then
  build_server="$(wslpath -w "$server_binary")"
  build_cli="$(wslpath -w "$cli_binary")"
fi
if ! (
  cd "$INSTALL_ROOT/app"
  GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local "$GO_BIN" list -mod=readonly -deps ./cmd/server ./cmd/tk >/dev/null
); then
  fail 'required Go modules are not available locally; fetch and verify them in the source checkout before running this offline bootstrap.' 69
fi
(
  cd "$INSTALL_ROOT/app"
  GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local "$GO_BIN" build -trimpath -o "$build_server" ./cmd/server
  GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local "$GO_BIN" build -trimpath -o "$build_cli" ./cmd/tk
)
[[ -s "$server_binary" && -s "$cli_binary" ]] || fail 'local Go build did not produce both Time Keeper binaries.'
chmod 700 "$server_binary" "$cli_binary"

cat > "$INSTALL_ROOT/timekeeper" <<LAUNCHER
#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
set -Eeuo pipefail
ROOT="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd -P)"
ADDR="\${TIMEKEEPER_ADDR:-127.0.0.1:1618}"
DB_PATH="\${TIMEKEEPER_DB:-\$ROOT/state/timekeeper.db}"
UI_PATH="\${TIMEKEEPER_UI:-\$ROOT/app/web/timekeeper.html}"
TARGET_GOOS='$GOOS'
if [[ "\$TARGET_GOOS" == 'windows' && -x "\$(command -v wslpath || true)" ]]; then
  DB_PATH="\$(wslpath -w "\$DB_PATH")"
  UI_PATH="\$(wslpath -w "\$UI_PATH")"
fi
exec "\$ROOT/bin/$(basename "$server_binary")" -addr "\$ADDR" -db "\$DB_PATH" -ui "\$UI_PATH"
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

mv "$INSTALL_ROOT" "$DESTINATION"
trap - EXIT
rmdir "$STAGE"
printf 'Time Keeper installed from pinned commit %s\n' "$COMMIT"
printf 'Installation root: %s\n' "$DESTINATION"
printf 'No server was started. Run: %s/timekeeper\n' "$DESTINATION"
printf 'Use the CLI after the server is running: %s/tk list\n' "$DESTINATION"
