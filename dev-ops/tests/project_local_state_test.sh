#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
for local_artifact in \
  .timekeeper/runtime-state-probe.db \
  timekeeper-backup.db \
  extensions/vscode/timekeeper-vscode-0.1.0.vsix; do
  git -C "$ROOT" check-ignore -q "$local_artifact" || {
    printf 'local artifact must remain untracked: %s\n' "$local_artifact" >&2
    exit 1
  }
done
