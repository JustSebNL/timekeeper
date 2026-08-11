#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
# INSTALLER_DISABLED_UNTIL_RELEASE_READINESS
set -Eeuo pipefail

printf '%s\n' \
  'Time Keeper installation is intentionally disabled.' \
  'Reason: a production installer requires a completed threat review, reproducible release artifacts, integrity verification, and platform-specific service validation.' \
  'Build and run from the checked-out source for now; see README.md and HELP.md.' >&2
exit 64
