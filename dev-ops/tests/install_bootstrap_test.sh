#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/install.sh"
EXPECTED_ORIGIN='git@github.com:JustSebNL/timekeeper.git'

[[ -f "$SCRIPT" ]] || { printf 'missing installer: %s\n' "$SCRIPT" >&2; exit 1; }
bash -n "$SCRIPT"
for required in \
  'SOURCE="$ROOT"' \
  'DESTINATION="$ROOT/.timekeeper/app"' \
  'archive --format=tar' \
  'SOURCE_COMMIT' \
  'GOPROXY=off' \
  'GOTOOLCHAIN=local' \
  "$EXPECTED_ORIGIN" \
  'preserving existing runtime state' \
  'Run: ./.timekeeper/app/timekeeper'; do
  grep -Fq -- "$required" "$SCRIPT" || {
    printf 'installer is missing repository-contained bootstrap contract: %s\n' "$required" >&2
    exit 1
  }
done
if grep -Eq -- '--source|--destination|\.local/share/timekeeper|LOCALAPPDATA' "$SCRIPT"; then
  printf 'installer exposes stale path-selection behavior\n' >&2
  exit 1
fi
if grep -Eq '(sudo |apt-get|dnf |yum |pacman |systemctl |useradd |curl |wget )' "$SCRIPT"; then
  printf 'installer must not require privileged, package-manager, service, or network commands\n' >&2
  exit 1
fi

TMP="$(mktemp -d)"
SOURCE="$TMP/source"
cleanup() {
  rm -rf "$TMP"
}
trap cleanup EXIT

git clone --no-hardlinks --local "$ROOT" "$SOURCE" >/dev/null
# Exercise the current installer implementation against an otherwise clean,
# self-contained clone without changing the active checkout's remote refs.
cp "$SCRIPT" "$SOURCE/install.sh"
git -C "$SOURCE" add install.sh
git -C "$SOURCE" -c user.name='Time Keeper test' -c user.email='timekeeper-test@example.invalid' commit -m 'test current installer' >/dev/null
git -C "$SOURCE" remote set-url origin "$EXPECTED_ORIGIN"
git -C "$SOURCE" update-ref refs/remotes/origin/main HEAD
COMMIT="$(git -C "$SOURCE" rev-parse HEAD)"

FAKE_GO="$TMP/go-tool"
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

TARGET="$SOURCE/.timekeeper/app"
(
  cd "$SOURCE"
  TIMEKEEPER_GO="$FAKE_GO" bash ./install.sh > "$TMP/install.out"
)
[[ -f "$TARGET/app/README.md" ]]
[[ -f "$TARGET/../web/index.html" ]]
[[ -f "$TARGET/../web/vendor/bootstrap-5.3.8.min.css" ]]
[[ -x "$TARGET/bin/timekeeper" ]]
[[ -x "$TARGET/bin/tk" ]]
[[ -x "$TARGET/timekeeper" ]]
[[ -x "$TARGET/tk" ]]
[[ ! -e "$TARGET/state/timekeeper.db" ]]
grep -Fqx "SOURCE_COMMIT=$COMMIT" "$TARGET/INSTALLATION.env"
grep -Fqx "SOURCE_ORIGIN=$EXPECTED_ORIGIN" "$TARGET/INSTALLATION.env"
grep -Fqx 'INSTALLER_STARTS_NO_SERVER=1' "$TARGET/INSTALLATION.env"

printf 'preserved-state' > "$TARGET/state/runtime-marker"
(
  cd "$SOURCE"
  TIMEKEEPER_GO="$FAKE_GO" bash ./install.sh > "$TMP/refresh.out"
)
[[ "$(<"$TARGET/state/runtime-marker")" == 'preserved-state' ]]
grep -Fq 'preserving existing runtime state' "$TMP/refresh.out"

if (
  cd "$SOURCE"
  TIMEKEEPER_GO="$FAKE_GO" bash ./install.sh --destination "$TMP/elsewhere"
) >/dev/null 2>&1; then
  printf 'installer accepted obsolete path-selection arguments\n' >&2
  exit 1
fi
[[ ! -e "$TMP/elsewhere" ]]

printf '\n' >> "$SOURCE/README.md"
if (
  cd "$SOURCE"
  TIMEKEEPER_GO="$FAKE_GO" bash ./install.sh
) >/dev/null 2>&1; then
  printf 'installer accepted a dirty source checkout\n' >&2
  exit 1
fi

printf 'install-bootstrap-contract=passed\n'
