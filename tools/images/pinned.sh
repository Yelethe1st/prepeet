#!/usr/bin/env bash
# Every FROM and every --from image is pinned by digest.
#
# PLT-02: an artifact whose inputs can change underneath it is not immutable.
# `golang:1.26-alpine` means something different next week, so a build that
# names a tag cannot say what it was built from and cannot be reproduced.
#
# Checked by reading the Dockerfiles rather than by trusting review, because
# this is exactly the kind of line that gets edited in a hurry when a base image
# needs bumping.
set -euo pipefail

broken=0
checked=0

while IFS= read -r dockerfile; do
  while IFS= read -r line; do
    # The image reference is the last field of a FROM, or the argument of a
    # --from=, before any AS alias.
    case "$line" in
      FROM\ *)
        image=$(printf '%s' "$line" | awk '{print $2}')
        ;;
      *--from=*)
        image=$(printf '%s' "$line" | sed -n 's/.*--from=\([^ ]*\).*/\1/p')
        ;;
      *) continue ;;
    esac
    # A stage alias rather than an image: nothing to pin.
    case "$image" in
      *:*|*/*) ;;
      *) continue ;;
    esac
    checked=$((checked + 1))
    case "$image" in
      *@sha256:*) ;;
      *)
        printf '\033[31mFAIL\033[0m %s names %s by tag rather than by digest\n' \
          "$dockerfile" "$image" >&2
        broken=1
        ;;
    esac
  done < "$dockerfile"
done < <(find services apps -name Dockerfile -not -path '*/node_modules/*' 2>/dev/null)

# A run that checked nothing is a failure rather than a pass, for the reason
# tools/docs/links.sh gives: a check that silently found no work to do is
# indistinguishable from one that passed.
if [ "$checked" -eq 0 ]; then
  printf '\033[31mFAIL\033[0m no base images were checked, so nothing was verified\n' >&2
  exit 1
fi

if [ "$broken" -ne 0 ]; then
  exit 1
fi

printf '\033[32mPASS\033[0m %d base images are pinned by digest\n' "$checked"
