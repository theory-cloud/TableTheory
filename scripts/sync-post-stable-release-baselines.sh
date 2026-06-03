#!/usr/bin/env bash
set -euo pipefail

# Syncs the stable release baseline from the current main checkout back to the
# integration/prerelease branches after a stable release is published.

remote="${SYNC_RELEASE_BASELINE_REMOTE:-origin}"
targets="${SYNC_RELEASE_BASELINE_TARGETS:-premain staging}"
push="${SYNC_RELEASE_BASELINE_PUSH:-false}"
retries="${SYNC_RELEASE_BASELINE_PUSH_RETRIES:-3}"

repo_root="$(git rev-parse --show-toplevel)"

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

release_files=(
  ".release-please-manifest.json"
  "CHANGELOG.md"
  "py/src/theorydb_py/version.json"
  "ts/package.json"
  "ts/package-lock.json"
)
sync_files=(
  "${release_files[@]}"
  ".release-please-manifest.premain.json"
)

for file in "${release_files[@]}" ".release-please-manifest.premain.json"; do
  if [[ ! -f "${repo_root}/${file}" ]]; then
    echo "release-baseline-sync: FAIL (missing ${file})"
    exit 1
  fi
done

tmp_root="$(mktemp -d)"
cleanup() {
  if [[ -d "${tmp_root}" ]]; then
    while IFS= read -r worktree; do
      git worktree remove --force "${worktree}" >/dev/null 2>&1 || true
    done < <(find "${tmp_root}" -mindepth 1 -maxdepth 1 -type d 2>/dev/null || true)
    rm -rf "${tmp_root}"
  fi
}
trap cleanup EXIT

push_remote="${remote}"
if [[ "${push}" == "true" && -n "${GH_TOKEN:-}" && -n "${GITHUB_REPOSITORY:-}" ]]; then
  push_remote="https://x-access-token:${GH_TOKEN}@github.com/${GITHUB_REPOSITORY}.git"
fi

copy_release_baseline() {
  local worktree="$1"

  for file in "${release_files[@]}"; do
    mkdir -p "${worktree}/$(dirname "${file}")"
    cp "${repo_root}/${file}" "${worktree}/${file}"
  done

  (
    cd "${worktree}"
    STABLE_VERSION="${stable_version}" python3 - <<'PY'
import json
import os
from pathlib import Path

Path(".release-please-manifest.premain.json").write_text(
    json.dumps({".": os.environ["STABLE_VERSION"]}, indent=2) + "\n",
    encoding="utf-8",
)
PY
  )
}

sync_branch() {
  local branch="$1"
  local attempt=1
  local safe_branch="${branch//\//_}"

  while (( attempt <= retries )); do
    local worktree="${tmp_root}/${safe_branch}-${attempt}"

    git fetch --quiet --depth=1 "${remote}" "${branch}:refs/remotes/${remote}/${branch}"
    git worktree add --quiet --detach "${worktree}" "${remote}/${branch}"

    copy_release_baseline "${worktree}"

    if git -C "${worktree}" diff --quiet -- "${sync_files[@]}"; then
      echo "release-baseline-sync: ${branch}: already at ${stable_version}"
      return 0
    fi

    echo "release-baseline-sync: ${branch}: syncing to ${stable_version}"
    git -C "${worktree}" diff --stat -- "${sync_files[@]}"

    if [[ "${push}" != "true" ]]; then
      echo "release-baseline-sync: ${branch}: dry-run only"
      return 0
    fi

    git -C "${worktree}" config user.name "github-actions[bot]"
    git -C "${worktree}" config user.email "41898282+github-actions[bot]@users.noreply.github.com"
    git -C "${worktree}" add -- "${sync_files[@]}"
    git -C "${worktree}" commit -m "chore(release): sync ${branch} baseline to ${stable_version}"

    if git -C "${worktree}" push "${push_remote}" "HEAD:refs/heads/${branch}"; then
      echo "release-baseline-sync: ${branch}: pushed ${stable_version}"
      return 0
    fi

    if (( attempt >= retries )); then
      echo "release-baseline-sync: FAIL (${branch} push failed after ${retries} attempts)"
      exit 1
    fi

    echo "release-baseline-sync: ${branch}: push raced; retrying (${attempt}/${retries})"
    git worktree remove --force "${worktree}" >/dev/null 2>&1 || true
    sleep "$((attempt * 2))"
    attempt=$((attempt + 1))
  done
}

for branch in ${targets}; do
  sync_branch "${branch}"
done

echo "release-baseline-sync: PASS (${stable_version})"
