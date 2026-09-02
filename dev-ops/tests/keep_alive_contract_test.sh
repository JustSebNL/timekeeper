#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"

python3 - "$ROOT" <<'PY'
from pathlib import Path
import sys

root = Path(sys.argv[1])
main = (root / "cmd/server/main.go").read_text()
sh = (root / "scripts/service/kick-server.sh").read_text()
bat = (root / "scripts/service/kick-server.bat").read_text()

for marker in [
    'keep-alive-interval',
    'func runKeepAlive',
    'func keepAliveOnce',
    '"/health"',
    'Timeout: 2 * time.Second',
]:
    assert marker in main, f"main.go missing keep-alive contract: {marker}"

for marker in [
    'curl -fsS --max-time 4',
    '"status":"ok"',
    'systemctl --user restart timekeeper.service',
]:
    assert marker in sh, f"kick-server.sh missing failure-only recovery contract: {marker}"
assert '--force-restart' in sh and '--no-update' in sh, 'kick-server.sh must use the no-update forced-recovery path'

for marker in [
    'WindowStyle Hidden',
    '/health',
    'restart TimeKeeper',
    'wsl.exe --exec bash',
]:
    assert marker in bat, f"kick-server.bat missing silent recovery contract: {marker}"

assert 'for _ in' not in sh, 'kick-server.sh must not become a continuous polling loop'
print('keep-alive-and-kick-contract=passed')
PY

bash -n "$ROOT/scripts/service/kick-server.sh"
bash -n "$ROOT/scripts/service/service-manager.sh"
