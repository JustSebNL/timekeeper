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

grep -Fqx 'tk t  new <project-id> <category-id> <task-name> <estimate>' HELP.md
grep -Fqx 'tk st new <task-id> <subtask-name> <estimate>' HELP.md
grep -Fqx 'tk sp new <task|subtask> <owner-id> <sprint-name> <estimate> [buffer-percent]' HELP.md
if grep -Fq '<estimate-seconds>' HELP.md; then
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
grep -Fqx 'GET  /api/v1/pulse' API.md
grep -Fq '## Pulse' API.md
grep -Fqx 'GET  /api/v1/projects/{projectID}/usage-summary' API.md
grep -Fqx 'POST /api/v1/projects/{projectID}/usage-sessions/{sessionID}/snapshots' API.md
grep -Fq '## Agent usage and token telemetry' API.md
grep -Fqx 'POST /api/v1/agents/{agentID}/progress' API.md
grep -Fqx 'GET  /api/v1/agents/{agentID}/nudges' API.md
grep -Fqx 'GET  /api/v1/agents/{agentID}/nudges/history' API.md
grep -Fqx 'POST /api/v1/agents/{agentID}/nudges/{nudgeID}/ack' API.md
grep -Fq 'timekeeper-pulse-guardian/v1' API.md
grep -Fq 'X-Timekeeper-Pulse-Accepted: v1' API.md
grep -Fqx 'tk pulse' HELP.md
grep -Fqx 'tk usage <project-id>' HELP.md
grep -Fq 'tk usage record <project-id> <session-id>' HELP.md
grep -Fqx 'tk agent progress <agent-id> <lease> [sprint-id] [guardian-url]' HELP.md
grep -Fqx 'tk agent history <agent-id>' HELP.md
if grep -Fq 'Pulses' HELP.md; then
  printf 'stale-pulse-unimplemented-reference-present\n' >&2
  exit 1
fi
grep -Fq 'duration_seconds' API.md
grep -Fq 'reason' API.md

for required_readme_line in \
  'Time Keeper helps an agent plan work, keep time, and remember what needs attention.' \
  '## What Time Keeper does' \
  '## Install and run' \
  '## When work is late' \
  '## Pulse' \
  'Pulse is a local attention check: it highlights Active Sprints only after they exceed their declared plan.' \
  '## Where it is going' \
  'Over time, Time Keeper should help agents see which models spend tokens without producing useful progress.'; do
  grep -Fqx "$required_readme_line" README.md || {
    printf 'README is missing clear agent-facing product guidance: %s\n' "$required_readme_line" >&2
    exit 1
  }
done
grep -Fq 'Accepts explicit cumulative agent usage snapshots' README.md
grep -Fq '## Agent usage snapshots' AGENT_INTEGRATION.md
grep -Fq 'TimeKeeper Agent Contract' TIMEKEEPER.md
grep -Fq 'harness/skills/' TIMEKEEPER.md
if grep -Eq '(^|[^[:alnum:]_])\./bin/(timekeeper|tk)|-db timekeeper\.db' README.md; then
  printf 'README still documents obsolete root-level runtime paths\n' >&2
  exit 1
fi

printf 'public-documentation-contract=passed\n'
