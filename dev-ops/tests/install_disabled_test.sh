#!/usr/bin/env bash
# Copyright (c) 2026 Seb. All rights reserved.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/install.sh"

[[ -x "$SCRIPT" || -f "$SCRIPT" ]] || { printf 'missing installer: %s\n' "$SCRIPT" >&2; exit 1; }
grep -Fq 'INSTALLER_DISABLED_UNTIL_RELEASE_READINESS' "$SCRIPT" || {
  printf 'installer must carry the release-readiness disable sentinel\n' >&2
  exit 1
}
grep -Fq 'exit 64' "$SCRIPT" || {
  printf 'installer must fail safely with EX_USAGE (64)\n' >&2
  exit 1
}
if grep -Eq '(apt-get|dnf |yum |pacman |curl |sudo |systemctl |useradd )' "$SCRIPT"; then
  printf 'disabled installer must not retain privileged or network side effects\n' >&2
  exit 1
fi
