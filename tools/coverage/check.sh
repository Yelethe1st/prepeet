#!/usr/bin/env bash
# Fails the build when a coverage percentage is below its threshold.
#
# Coverage is a floor, not a goal. A high number proves nothing about whether
# the invariants are tested, which is why docs/delivery/tickets/README.md also
# requires the named edge cases. What this gate catches is coverage silently
# eroding as the codebase grows.
#
# Usage: check.sh <label> <actual-percent> <threshold-percent>
set -euo pipefail

label="${1:?usage: check.sh <label> <actual> <threshold>}"
actual="${2:?usage: check.sh <label> <actual> <threshold>}"
threshold="${3:?usage: check.sh <label> <actual> <threshold>}"

# awk rather than bash arithmetic, because coverage is fractional.
if awk -v a="$actual" -v t="$threshold" 'BEGIN { exit !(a + 0 < t + 0) }'; then
  printf '\033[31mFAIL\033[0m %s coverage %s%% is below the %s%% threshold\n' "$label" "$actual" "$threshold" >&2
  exit 1
fi

printf '\033[32mPASS\033[0m %s coverage %s%% meets the %s%% threshold\n' "$label" "$actual" "$threshold"
