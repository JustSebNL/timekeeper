#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
# RELEASE_PREFLIGHT_VALIDATES_LOCAL_BOOTSTRAP
#
# This validates a checked-out source tree and its disposable local bootstrap
# harness. It performs no network or privileged action, does not modify PATH,
# and does not establish service or remote-deployment readiness.
set -Eeuo pipefail
umask 077

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_BIN="${TIMEKEEPER_GO:-go}"
WORK_ROOT="$ROOT/.timekeeper"

if ! "$GO_BIN" version >/dev/null 2>&1; then
  printf 'Time Keeper release preflight requires Go. Set TIMEKEEPER_GO to a Go executable.\n' >&2
  exit 69
fi
if ! command -v node >/dev/null 2>&1; then
  printf 'Time Keeper release preflight requires Node.js to syntax-check the dashboard script.\n' >&2
  exit 69
fi
for asset in web/index.html web/timekeeper.css web/timekeeper.js web/vendor/bootstrap-5.3.8.min.css; do
  [[ -f "$ROOT/$asset" && -r "$ROOT/$asset" ]] || { printf 'Missing dashboard asset: %s\n' "$asset" >&2; exit 66; }
done
node --check "$ROOT/web/timekeeper.js"
bash "$ROOT/dev-ops/tests/project_local_state_test.sh"
bash "$ROOT/dev-ops/tests/project_content_scope_test.sh"
bash "$ROOT/dev-ops/tests/copyright_attribution_contract_test.sh"
bash "$ROOT/dev-ops/tests/public_documentation_contract_test.sh"
bash "$ROOT/dev-ops/tests/install_bootstrap_test.sh"
bash "$ROOT/dev-ops/tests/install_e2e_test.sh"
bash "$ROOT/dev-ops/tests/keep_alive_contract_test.sh"
bash "$ROOT/dev-ops/tests/harness_contract_test.sh"

mkdir -p "$WORK_ROOT"
chmod 700 "$WORK_ROOT"
WORK_DIR="$(mktemp -d "$WORK_ROOT/release-preflight.XXXXXX")"
trap 'rm -rf "$WORK_DIR"' EXIT

cd "$ROOT"
"$GO_BIN" test ./... -count=1
"$GO_BIN" vet ./...
tracked_files="$(git ls-files)"
if [[ -n "$tracked_files" ]]; then
  git diff --check
else
  printf 'release-preflight: tracked-files=none; git-diff-check=skipped (no VCS baseline)\n' >&2
fi

GOOS="$("$GO_BIN" env GOOS)"
SERVER="$WORK_DIR/timekeeper"
CLI="$WORK_DIR/tk"
[[ "$GOOS" == windows ]] && { SERVER+=".exe"; CLI+=".exe"; }
BUILD_SERVER="$SERVER"
BUILD_CLI="$CLI"
if [[ "$GOOS" == windows ]] && command -v wslpath >/dev/null 2>&1; then
  BUILD_SERVER="$(wslpath -w "$SERVER")"
  BUILD_CLI="$(wslpath -w "$CLI")"
fi
"$GO_BIN" build -trimpath -o "$BUILD_SERVER" ./cmd/server
"$GO_BIN" build -trimpath -o "$BUILD_CLI" ./cmd/tk
[[ -s "$SERVER" && -s "$CLI" ]] || { printf 'Release preflight build artifacts are missing.\n' >&2; exit 70; }
printf 'release-preflight=passed (source and disposable local-bootstrap validation; no deployment or service readiness claim)\n'
