#!/usr/bin/env bash
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/launcher.sh"

python3 - "$SCRIPT" <<'PY'
from pathlib import Path
import re
import sys

text = Path(sys.argv[1]).read_text()
assert 'curl -fsS --max-time 4 "http://$ADDR/health"' in text, 'launcher must use failing health probe'
assert 'case "$health_payload" in' in text, 'launcher must inspect health payload'
assert '*\'"status":"ok"\'*) return 0 ;;' in text, 'launcher must require status=ok'
assert 'if [ "$FORCE_RESTART" -eq 0 ] && is_healthy; then' in text, 'healthy path must short-circuit unless explicitly forced'
health_end = text.index('if [ "$FORCE_RESTART" -eq 0 ] && is_healthy; then')
recovery_start = text.index('kill_port "$PORT"')
assert health_end < recovery_start, 'health gate must precede all process-kill/recovery work'
assert 'curl -s -o /dev/null --max-time 4 "http://$ADDR/"' not in text, 'launcher must not use weak root probe'
assert 'kill_port "1621"' not in text, 'recovery must not kill a separate legacy instance'
assert 'for _ in $(seq 1 10); do' in text, 'startup verification must retry readiness'
assert 'if is_healthy; then' in text, 'startup verification must reuse health predicate'
print('PASS: TimeKeeper launcher is idempotent and health-gated')
PY

bash -n "$SCRIPT"
printf 'PASS: launcher shell syntax\n'
