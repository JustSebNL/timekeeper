#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

if [[ -e SKILL.md || -d skills ]]; then
  printf 'personal-skill-material-present-at-project-root\n' >&2
  exit 1
fi

while IFS= read -r path; do
  case "$path" in
    SKILL.md|*/SKILL.md|skills/*|*/skills/*)
      printf 'tracked-skill-material-is-not-a-project-harness=%s\n' "$path" >&2
      exit 1
      ;;
  esac
done < <(git ls-files)

gem_word='gem'
mining_word='mining'
mount_root='/mnt'
external_repo_shelf="$mount_root/h/dev/repos"
forbidden_pattern="${gem_word}[ -]?${mining_word}|vault_snippets|track_${gem_word}s|$external_repo_shelf"
if matches="$(git grep -IlE "$forbidden_pattern" -- . ':!dev-ops/tests/project_content_scope_test.sh' || true)" && [[ -n "$matches" ]]; then
  printf 'unrelated-mining-material-present-in-tracked-source:\n%s\n' "$matches" >&2
  exit 1
fi

printf 'project-content-scope=passed\n'
