#!/usr/bin/env bash
#
# pending_commits.sh — report the commits not yet documented in RELEASE.md.
#
# It figures out two things and prints them for the skill to consume:
#   - current_tag: the latest tag reachable from HEAD (the version to document)
#   - documented:  the most recent version already written at the top of
#                  RELEASE.md (its first "## vX.Y.Z" heading)
# then lists every non-merge commit in the range documented..current_tag, so the
# release notes only ever cover what's genuinely new.
#
# Usage: pending_commits.sh [path/to/RELEASE.md]   (defaults to RELEASE.md)

set -euo pipefail

release_file="${1:-RELEASE.md}"

current_tag="$(git describe --tags --abbrev=0 2>/dev/null || true)"

documented=""
if [ -f "$release_file" ]; then
  documented="$(grep -m1 -oE '^##[[:space:]]+v[0-9][^[:space:]]*' "$release_file" \
    | sed -E 's/^##[[:space:]]+//' || true)"
fi

echo "current_tag=${current_tag:-<none>}"
echo "documented=${documented:-<none>}"

# A documented version that no longer resolves to a real ref (e.g. a hand-typed
# heading) would break the range, so fall back to the full history in that case.
if [ -n "$documented" ] && ! git rev-parse -q --verify "refs/tags/${documented}" >/dev/null 2>&1; then
  echo "warning: '${documented}' from ${release_file} is not a tag; covering full history instead" >&2
  documented=""
fi

if [ -n "$documented" ]; then
  range="${documented}..${current_tag:-HEAD}"
else
  range="${current_tag:-HEAD}"
fi
echo "range=${range}"
echo

if [ -n "$documented" ] && [ "$documented" = "$current_tag" ]; then
  echo "(RELEASE.md already documents ${current_tag}; nothing to add)"
  exit 0
fi

echo "commits:"
git log --no-merges --reverse --pretty=format:'%h%x09%s' "$range"
echo
