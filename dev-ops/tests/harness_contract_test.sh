#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
python3 - "$ROOT" <<'PY'
from pathlib import Path
import sys

root = Path(sys.argv[1])
required = [
    root / "TIMEKEEPER.md",
    root / "harness/manifest.yaml",
    root / "harness/SKILL.md",
    root / "harness/skills/project-landscape/SKILL.md",
    root / "harness/skills/backlog-sync/SKILL.md",
    root / "harness/skills/codebase-audit/SKILL.md",
    root / "harness/skills/timeboxing/SKILL.md",
    root / "harness/skills/troubleshooting/SKILL.md",
    root / "scripts/timekeeper-harness.sh",
    root / "scripts/timekeeper-harness.bat",
]
for path in required:
    assert path.is_file() and path.stat().st_size > 0, f"missing harness artifact: {path}"

contract = (root / "TIMEKEEPER.md").read_text()
for marker in ["tk doctor", "Project, Category, Task, and Sprint", "scripts/timekeeper-harness.sh", "scripts/timekeeper-harness.bat"]:
    assert marker in contract, f"TIMEKEEPER.md missing {marker}"

manifest = (root / "harness/manifest.yaml").read_text()
for marker in ["project_landscape", "bounded_sprint", "evidence_before_completion", "healthy_path: no-op"]:
    assert marker in manifest, f"manifest missing {marker}"

sh = (root / "scripts/timekeeper-harness.sh").read_text()
for marker in ["/health", "scripts/service/kick-server.sh", "for _ in $(seq 1 10)"]:
    assert marker in sh, f"harness shell missing {marker}"
assert "if healthy; then" in sh

bat = (root / "scripts/timekeeper-harness.bat").read_text()
for marker in ["WindowStyle Hidden", "/health", "service\\kick-server.bat"]:
    assert marker in bat, f"harness batch missing {marker}"

print("agent-harness-contract=passed")
PY
bash -n "$ROOT/scripts/timekeeper-harness.sh"
bash -n "$ROOT/scripts/service/kick-server.sh"
