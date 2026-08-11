#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
set -Eeuo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

for obsolete in timekeeper.html project.md project-adopt.md; do
  if [[ -e "$obsolete" ]]; then
    printf 'obsolete-public-artifact-present=%s\n' "$obsolete" >&2
    exit 1
  fi
done

grep -Fqx 'tk t new <project-id> <category-id> <task-name> <estimate>' SKILL.md
grep -Fqx 'tk st new <task-id> <subtask-name> <estimate>' SKILL.md
grep -Fqx 'tk sp new task <task-id> <sprint-name> <estimate> [buffer-percent]' SKILL.md
grep -Fqx 'tk sp new subtask <subtask-id> <sprint-name> <estimate> [buffer-percent]' SKILL.md
if grep -Fq '<estimate-seconds>' SKILL.md; then
  printf 'stale-sprint-estimate-syntax-present\n' >&2
  exit 1
fi
if grep -Fq '## Planned CLI behavior' HELP.md; then
  printf 'stale-planned-cli-reference-present\n' >&2
  exit 1
fi

grep -Fqx 'GET  /api/v1/sprints/{sprintID}/extensions' API.md
grep -Fqx 'POST /api/v1/sprints/{sprintID}/extensions' API.md
grep -Fq '## Sprint extensions' API.md
grep -Fq 'duration_seconds' API.md
grep -Fq 'reason' API.md

printf 'public-documentation-contract=passed\n'
