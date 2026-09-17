#!/usr/bin/env bash
set -Eeuo pipefail

# Add/install TimeKeeper as a Linux systemd user service.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "$SCRIPT_DIR/../scripts/service/service-manager.sh" install "$@"
