#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/install.sh"
EXPECTED_ORIGIN='git@github.com:JustSebNL/timekeeper.git'

[[ -f "$SCRIPT" ]] || { printf 'missing installer: %s\n' "$SCRIPT" >&2; exit 1; }
bash -n "$SCRIPT"
for required in \
  '--source' \
  '--destination' \
  'archive --format=tar' \
  'SOURCE_COMMIT' \
  'GOPROXY=off' \
  'GOTOOLCHAIN=local' \
  "$EXPECTED_ORIGIN" \
  '.local/share/timekeeper' \
  'LOCALAPPDATA' \
  'refusing non-empty destination' \
  'must be clean'; do
  grep -Fq -- "$required" "$SCRIPT" || {
    printf 'installer is missing required safety contract: %s\n' "$required" >&2
    exit 1
  }
done
if grep -Eq '(sudo |apt-get|dnf |yum |pacman |systemctl |useradd |curl |wget )' "$SCRIPT"; then
  printf 'installer must not require privileged, package-manager, service, or network commands\n' >&2
  exit 1
fi

TMP="$(mktemp -d)"
SOURCE="$TMP/source"
TARGET="$TMP/installed"
cleanup() {
  git -C "$ROOT" worktree remove --force "$SOURCE" >/dev/null 2>&1 || true
  rm -rf "$TMP"
}
trap cleanup EXIT

git -C "$ROOT" worktree add --detach "$SOURCE" origin/main >/dev/null
COMMIT="$(git -C "$SOURCE" rev-parse HEAD)"
FAKE_GO="$TMP/go tool"
cat > "$FAKE_GO" <<'FAKE_GO'
#!/usr/bin/env bash
set -Eeuo pipefail
case "${1:-}" in
  version) printf 'go version fake\n'; exit 0 ;;
  env) [[ "${2:-}" == 'GOOS' ]]; printf 'linux\n'; exit 0 ;;
  list) exit 0 ;;
  build)
    output=''
    while (($#)); do
      if [[ "$1" == '-o' ]]; then
        output="$2"
        break
      fi
      shift
    done
    [[ -n "$output" ]]
    mkdir -p "$(dirname "$output")"
    printf '#!/usr/bin/env bash\nexit 0\n' > "$output"
    chmod 0755 "$output"
    exit 0
    ;;
esac
exit 1
FAKE_GO
chmod 0755 "$FAKE_GO"

TIMEKEEPER_GO="$FAKE_GO" "$SCRIPT" --source "$SOURCE" --destination "$TARGET" > "$TMP/install.out"
[[ -f "$TARGET/app/README.md" ]]
[[ -x "$TARGET/bin/timekeeper" ]]
[[ -x "$TARGET/bin/tk" ]]
[[ -x "$TARGET/timekeeper" ]]
[[ -x "$TARGET/tk" ]]
[[ ! -e "$TARGET/state/timekeeper.db" ]]
grep -Fqx "SOURCE_COMMIT=$COMMIT" "$TARGET/INSTALLATION.env"
grep -Fqx "SOURCE_ORIGIN=$EXPECTED_ORIGIN" "$TARGET/INSTALLATION.env"
grep -Fqx 'INSTALLER_STARTS_NO_SERVER=1' "$TARGET/INSTALLATION.env"

mkdir "$TMP/nonempty"
touch "$TMP/nonempty/keep"
if TIMEKEEPER_GO="$FAKE_GO" "$SCRIPT" --source "$SOURCE" --destination "$TMP/nonempty" >/dev/null 2>&1; then
  printf 'installer accepted a non-empty destination\n' >&2
  exit 1
fi

printf '\n' >> "$SOURCE/README.md"
if TIMEKEEPER_GO="$FAKE_GO" "$SCRIPT" --source "$SOURCE" --destination "$TMP/dirty" >/dev/null 2>&1; then
  printf 'installer accepted a dirty source checkout\n' >&2
  exit 1
fi
[[ ! -e "$TMP/dirty" ]]

printf 'install-bootstrap-contract=passed\n'
