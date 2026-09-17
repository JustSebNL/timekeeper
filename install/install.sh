#!/usr/bin/env bash
set -Eeuo pipefail

# Canonical Linux entrypoint. The implementation remains in the repository
# root for compatibility with existing release/bootstrap checks.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
exec bash "$SCRIPT_DIR/../install.sh" "$@"
