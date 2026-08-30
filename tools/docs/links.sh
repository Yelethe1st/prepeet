#!/usr/bin/env bash
# Fails when an internal documentation link points at a file that is not there.
#
# The documentation is binding: a ticket cites the spec it implements and an ADR
# cites the decision it supersedes, so a moved or renamed file quietly turns a
# citation into a dead end, and a dead citation is how a rule stops being read.
#
# This lived inside the CI workflow, which meant it could only be run by pushing
# and reading a log. It is a make target now, for the same reason the lint and
# coverage gates are: a check that exists in one copy, invoked the same way in
# both places, is a check that cannot quietly stop running in one of them.
#
# Only local links are followed. An external URL is somebody else's uptime, and
# a gate that fails when a third party has a bad morning gets disabled.
#
# Usage: links.sh [root ...]   (defaults to the documentation trees)
set -euo pipefail

roots=("$@")
if [ ${#roots[@]} -eq 0 ]; then
  roots=(docs screens/docs)
fi

broken=0
checked=0

while IFS= read -r file; do
  dir=$(dirname "$file")
  while IFS= read -r link; do
    # Anything that is not a path into the repository: absolute URLs, mail
    # addresses, and same-page anchors.
    case "$link" in http*|mailto:*|\#*) continue;; esac
    # A link may carry a fragment. The file is what can be missing; a heading
    # that moved is not something this can see without parsing every document.
    target="${link%%#*}"
    [ -z "$target" ] && continue
    checked=$((checked + 1))
    if [ ! -e "$dir/$target" ]; then
      echo "broken: $file -> $link"
      broken=1
    fi
  done < <(grep -o ']([^)]*)' "$file" | sed 's/](//;s/)$//')
done < <(find "${roots[@]}" -name '*.md')

# A run that checked nothing is a failure rather than a pass. The find above has
# been wrong before in exactly this way: point it at a directory that no longer
# exists and it reports success over an empty list, which is indistinguishable
# from every link resolving.
if [ "$checked" -eq 0 ]; then
  printf '\033[31mFAIL\033[0m no documentation links were checked, so nothing was verified\n' >&2
  exit 1
fi

if [ "$broken" -ne 0 ]; then
  printf '\033[31mFAIL\033[0m documentation links above do not resolve\n' >&2
  exit 1
fi

printf '\033[32mPASS\033[0m %d internal documentation links resolve\n' "$checked"
