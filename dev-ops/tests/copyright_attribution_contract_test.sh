#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COPYRIGHT='Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.'
OLD_HOLDER='Seb'

if git -C "$ROOT" grep -nF "Copyright (c) 2026 $OLD_HOLDER" -- ':!go.sum' ':!extensions/vscode/package-lock.json'; then
  printf 'stale Seb copyright attribution is present\n' >&2
  exit 1
fi

while IFS= read -r path; do
  [[ -n "$path" ]] || continue
  if ! grep -Fq "$COPYRIGHT" "$ROOT/$path"; then
    printf 'missing canonical copyright attribution: %s\n' "$path" >&2
    exit 1
  fi
done < <(git -C "$ROOT" ls-files '*.go' '*.js' '*.sh' '*.html' '*.css')

for legal_file in LICENSE NOTICE README.md HELP.md AGENT_INTEGRATION.md extensions/vscode/LICENSE extensions/vscode/README.md; do
  if ! grep -Fq "$COPYRIGHT" "$ROOT/$legal_file"; then
    printf 'missing canonical legal/document attribution: %s\n' "$legal_file" >&2
    exit 1
  fi
done

printf 'copyright-attribution-contract=passed\n'
