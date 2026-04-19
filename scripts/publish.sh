#!/usr/bin/env bash
# Bump VERSION (patch|minor|major), commit all, tag vX.Y.Z, push branch + tag.
# Preflight: make publish-verify (fmt-check, vet, test, build).
#
# Usage:
#   ./scripts/publish.sh           # default: minor
#   ./scripts/publish.sh patch
#   ./scripts/publish.sh major
# Or: make publish   /  make publish BUMP=patch
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BUMP="${1:-${BUMP:-minor}}"
case "$BUMP" in
patch | minor | major) ;;
*)
  echo "publish: BUMP must be patch, minor, or major (got '$BUMP')" >&2
  exit 1
  ;;
esac

make publish-verify

VERSION_FILE="${VERSION_FILE:-VERSION}"
if [[ ! -f "$VERSION_FILE" ]]; then
  echo "publish: missing $VERSION_FILE" >&2
  exit 1
fi

line="$(tr -d '\r\n' < "$VERSION_FILE")"
if [[ $(echo "$line" | awk -F. '{print NF}') -ne 3 ]]; then
  echo "publish: $VERSION_FILE must be exactly semver x.y.z (got '$line')" >&2
  exit 1
fi
IFS=. read -r MA MI PA <<< "$line"
if [[ -z "${MA:-}" || -z "${MI:-}" || -z "${PA:-}" ]]; then
  echo "publish: $VERSION_FILE must be semver x.y.z (got '$line')" >&2
  exit 1
fi

MA=$((10#$MA))
MI=$((10#$MI))
PA=$((10#$PA))

case "$BUMP" in
patch)
  PA=$((PA + 1))
  NEW_VER="${MA}.${MI}.${PA}"
  ;;
minor)
  MI=$((MI + 1))
  NEW_VER="${MA}.${MI}.0"
  ;;
major)
  MA=$((MA + 1))
  NEW_VER="${MA}.0.0"
  ;;
esac

echo "$NEW_VER" >"$VERSION_FILE"

if git rev-parse "refs/tags/v${NEW_VER}" >/dev/null 2>&1; then
  echo "publish: tag v${NEW_VER} already exists" >&2
  exit 1
fi

git add -A
if git diff --cached --quiet; then
  echo "publish: nothing staged after version bump (unexpected)" >&2
  exit 1
fi

git commit -m "chore: release v${NEW_VER}"
git tag -a "v${NEW_VER}" -m "v${NEW_VER}"

branch="$(git branch --show-current)"
git push origin "$branch"
git push origin "v${NEW_VER}"

echo "publish: pushed v${NEW_VER} (branch ${branch}, bump=${BUMP})"
