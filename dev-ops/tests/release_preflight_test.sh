#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/release-preflight.sh"

[[ -f "$SCRIPT" ]] || { printf 'missing release preflight: %s\n' "$SCRIPT" >&2; exit 1; }
bash -n "$SCRIPT"
for required in \
  'RELEASE_PREFLIGHT_DOES_NOT_ENABLE_INSTALLATION' \
  '"$GO_BIN" test ./... -count=1' \
  '"$GO_BIN" vet ./...' \
  'node --check "$ROOT/web/timekeeper.js"' \
  'project_local_state_test.sh' \
  'copyright_attribution_contract_test.sh' \
  'public_documentation_contract_test.sh' \
  'git diff --check' \
  'git ls-files' \
  'tracked-files=none; git-diff-check=skipped' \
  '"$GO_BIN" build -trimpath' \
  'INSTALLER_DISABLED_UNTIL_RELEASE_READINESS' \
  'install_status -eq 64'; do
  grep -Fq "$required" "$SCRIPT" || { printf 'release preflight is missing required gate: %s\n' "$required" >&2; exit 1; }
done
if grep -Eq '(sudo |apt-get|dnf |yum |pacman |systemctl |useradd |curl )' "$SCRIPT"; then
  printf 'release preflight must not require privileged, package-manager, service, or network commands\n' >&2
  exit 1
fi
