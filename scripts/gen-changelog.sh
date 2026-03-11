#!/bin/bash
set -euo pipefail

PREV_TAG=$(git describe --tags --abbrev=0 HEAD^ 2>/dev/null || true)
if [ -n "$PREV_TAG" ]; then
  BASE="$PREV_TAG"
else
  BASE="v0.0.0"
fi

{
  echo "## Changes since ${BASE}"
  echo
  if [ -n "$PREV_TAG" ]; then
    git log "${PREV_TAG}..HEAD" --pretty=format:"- %s (%an)"
  else
    git log --pretty=format:"- %s (%an)"
  fi
} > changelog.md

cat changelog.md
