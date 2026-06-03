#!/usr/bin/env bash
set -euo pipefail

# DEPRECATED / OPERATOR-ONLY DRY RUN.
# The release lane is staging -> premain -> main -> staging. After a stable
# release publishes on main, CI must not push baseline sync commits to premain
# or staging. The next operator step is a normal PR backmerge from main to
# staging; premain receives that baseline through the next staging -> premain
# promotion.

stable_version="$(
  python3 - <<'PY'
import json
from pathlib import Path

data = json.loads(Path(".release-please-manifest.json").read_text(encoding="utf-8"))
print(data.get(".", ""))
PY
)"

if [[ -z "${stable_version}" ]]; then
  echo "release-baseline-sync: FAIL (missing stable version in .release-please-manifest.json)"
  exit 1
fi

if [[ -n "${RELEASE_TAG_NAME:-}" ]]; then
  tag_version="${RELEASE_TAG_NAME#v}"
  if [[ "${tag_version}" != "${stable_version}" ]]; then
    echo "release-baseline-sync: FAIL (tag ${RELEASE_TAG_NAME} does not match manifest ${stable_version})"
    exit 1
  fi
fi

if [[ "${SYNC_RELEASE_BASELINE_PUSH:-false}" == "true" ]]; then
  echo "release-baseline-sync: FAIL (deprecated helper is dry-run only; use a normal main -> staging PR)"
  exit 1
fi

required_files=(
  ".release-please-manifest.json"
  ".release-please-manifest.premain.json"
  "CHANGELOG.md"
  "py/src/theorydb_py/version.json"
  "ts/package.json"
  "ts/package-lock.json"
)

for file in "${required_files[@]}"; do
  if [[ ! -f "${file}" ]]; then
    echo "release-baseline-sync: FAIL (missing ${file})"
    exit 1
  fi
done

echo "release-baseline-sync: DEPRECATED dry-run only"
echo "release-baseline-sync: stable=${stable_version}"
echo "release-baseline-sync: use a normal PR backmerge from main to staging; do not sync premain directly"
echo "release-baseline-sync: checked files: ${required_files[*]}"
echo "release-baseline-sync: PASS"
