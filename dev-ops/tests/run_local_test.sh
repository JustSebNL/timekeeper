#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/run-local.sh"

[[ -f "$SCRIPT" ]] || { printf 'missing portable launcher: %s\n' "$SCRIPT" >&2; exit 1; }
bash -n "$SCRIPT"
grep -Fq 'PORTABLE_REPO_LOCAL_LAUNCHER' "$SCRIPT" || {
  printf 'portable launcher sentinel missing\n' >&2
  exit 1
}
grep -Fq '.timekeeper' "$SCRIPT" || {
  printf 'portable launcher must retain local state under the repository\n' >&2
  exit 1
}
grep -Fq 'wslpath -w "$binary"' "$SCRIPT" || {
  printf 'portable launcher must translate Windows Go output paths under WSL\n' >&2
  exit 1
}
grep -Fq 'wslpath -w "$UI_PATH"' "$SCRIPT" || {
  printf 'portable launcher must translate dashboard paths for Windows Go under WSL\n' >&2
  exit 1
}
grep -Fq 'umask 077' "$SCRIPT" || {
  printf 'portable launcher must create local state with owner-only permissions\n' >&2
  exit 1
}
grep -Fq 'chmod 700 "$STATE_DIR"' "$SCRIPT" || {
  printf 'portable launcher must protect an existing local state directory\n' >&2
  exit 1
}
if grep -Eq '(sudo |apt-get|dnf |yum |pacman |systemctl |useradd |curl )' "$SCRIPT"; then
  printf 'portable launcher must not require privileged, package-manager, service, or network commands\n' >&2
  exit 1
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
FAKE_GO="$TMP/go tool"
cat > "$FAKE_GO" <<'FAKE'
#!/usr/bin/env bash
set -Eeuo pipefail
case "${1:-}" in
  version) exit 0 ;;
  env) printf 'linux\n'; exit 0 ;;
  build)
    [[ "$2" == "-o" ]]
    mkdir -p "$(dirname "$3")"
    printf '#!/usr/bin/env bash\nexit 0\n' > "$3"
    chmod 0755 "$3"
    exit 0
    ;;
esac
exit 1
FAKE
chmod 0755 "$FAKE_GO"
printf '<!doctype html>' > "$TMP/ui.html"
TIMEKEEPER_GO="$FAKE_GO" TIMEKEEPER_STATE_DIR="$TMP/state" TIMEKEEPER_UI="$TMP/ui.html" "$SCRIPT" >/dev/null
