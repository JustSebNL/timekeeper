#!/usr/bin/env bash
set -Eeuo pipefail

# Remove/uninstall TimeKeeper's Linux systemd user service.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "$SCRIPT_DIR/../scripts/service/service-manager.sh" uninstall "$@"
