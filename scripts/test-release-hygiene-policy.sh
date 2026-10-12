#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
checker="${repo_root}/scripts/verify-release-lane-provenance.sh"
base_sha="1111111111111111111111111111111111111111"
head_sha="2222222222222222222222222222222222222222"
advanced_base_sha="3333333333333333333333333333333333333333"
repo="theory-cloud/TableTheory"
tmpdirs=()

cleanup() {
  local tmpdir
  for tmpdir in "${tmpdirs[@]}"; do
    rm -rf "${tmpdir}"
  done
}
trap cleanup EXIT

expect_success_contains() {
  local expected="$1"
  shift

  local output
  if ! output="$("$@" 2>&1)"; then
    printf '%s\n' "${output}"
    echo "release-hygiene-policy-test: expected command to succeed"
    exit 1
  fi
  if ! grep -Fq "${expected}" <<<"${output}"; then
    printf '%s\n' "${output}"
    echo "release-hygiene-policy-test: expected output to contain: ${expected}"
    exit 1
  fi
}

expect_failure_contains() {
  local expected="$1"
  shift

  local output
  if output="$("$@" 2>&1)"; then
    printf '%s\n' "${output}"
    echo "release-hygiene-policy-test: expected command to fail"
    exit 1
  fi
  if ! grep -Fq "${expected}" <<<"${output}"; then
    printf '%s\n' "${output}"
    echo "release-hygiene-policy-test: expected failure output to contain: ${expected}"
    exit 1
  fi
}

expect_contains() {
  local output="$1"
  local expected="$2"
  local context="$3"

  if [[ -n "${expected}" ]] && ! grep -Fq "${expected}" <<<"${output}"; then
    printf '%s\n' "${output}"
    echo "release-hygiene-policy-test: ${context}: expected output to contain: ${expected}"
    exit 1
  fi
}

write_prerelease_postcondition_fixture() {
  local root="$1"
  local version="$2"
  local title="${3:-chore(premain): release ${version}}"
  local include_manifest="${4:-true}"
  local include_changelog="${5:-true}"
  local manifest_version="${6:-${version}}"

  mkdir -p "${root}"
  cat >"${root}/open-prs.json" <<JSON
[
  {
    "number": 353,
    "title": "${title}",
    "headRefName": "release-please--branches--premain",
    "url": "https://github.example.test/theory-cloud/TableTheory/pull/353"
  }
]
JSON

  python3 - "${root}/details.json" "${title}" "${include_manifest}" "${include_changelog}" <<'PY'
import json
import sys
from pathlib import Path

path, title, include_manifest, include_changelog = sys.argv[1:5]
files = []
if include_manifest == "true":
    files.append({"path": ".release-please-manifest.json"})
if include_changelog == "true":
    files.append({"path": "CHANGELOG.md"})
Path(path).write_text(
    json.dumps(
        {
            "number": 353,
            "title": title,
            "headRefName": "release-please--branches--premain",
            "baseRefName": "premain",
            "headRefOid": "2222222222222222222222222222222222222222",
            "url": "https://github.example.test/theory-cloud/TableTheory/pull/353",
            "files": files,
        }
    ),
    encoding="utf-8",
)
PY

  printf '{".":"%s"}\n' "${manifest_version}" >"${root}/manifest.json"
}

run_prerelease_postcondition_fixture() {
  local fixture="$1"
  shift

  bash "${repo_root}/scripts/verify-prerelease-pr-postcondition.sh" \
    --repo "${repo}" \
    --base premain \
    --open-prs-file "${fixture}/open-prs.json" \
    --details-file "${fixture}/details.json" \
    --manifest-file "${fixture}/manifest.json" \
    --dry-run \
    "$@"
}

write_package_version_fixture() {
  local root="$1"
  local version="${2:-1.10.1}"

  mkdir -p "${root}/ts" "${root}/py/src/tabletheory_py"
  printf '{"version":"%s"}\n' "${version}" >"${root}/ts/package.json"
  printf '{"version":"%s","packages":{"":{"version":"%s"}}}\n' \
    "${version}" "${version}" >"${root}/ts/package-lock.json"
  printf '{"version":"%s"}\n' "${version}" >"${root}/py/src/tabletheory_py/version.json"
}

assert_package_version_fixture() {
  local root="$1"
  local version="$2"

  python3 - "${root}" "${version}" <<'PY'
import json
import sys
from pathlib import Path

root = Path(sys.argv[1])
expected = sys.argv[2]

checks = [
    ("ts/package.json", json.loads((root / "ts/package.json").read_text())["version"]),
    ("ts/package-lock.json", json.loads((root / "ts/package-lock.json").read_text())["version"]),
    (
        "ts/package-lock.json packages['']",
        json.loads((root / "ts/package-lock.json").read_text())["packages"][""]["version"],
    ),
    (
        "py/src/tabletheory_py/version.json",
        json.loads((root / "py/src/tabletheory_py/version.json").read_text())["version"],
    ),
]
for label, actual in checks:
    if actual != expected:
        print(f"release-hygiene-policy-test: {label} = {actual!r}, expected {expected!r}")
        raise SystemExit(1)
PY
}

write_release_asset_fixture() {
  local root="$1"
  local version="$2"

  python3 - "${root}" "${version}" <<'PY'
import io
import json
import re
import sys
import tarfile
import zipfile
from pathlib import Path

root = Path(sys.argv[1])
version = sys.argv[2]
root.mkdir(parents=True, exist_ok=True)

match = re.fullmatch(r"(\d+\.\d+\.\d+)-rc(?:\.(\d+))?", version)
pep440 = f"{match.group(1)}rc{match.group(2) or '0'}" if match else version
version_json = json.dumps({"version": version}).encode("utf-8")

npm_package = json.dumps(
    {"name": "@theory-cloud/tabletheory-ts", "version": version},
    separators=(",", ":"),
).encode("utf-8")
with tarfile.open(root / f"theory-cloud-tabletheory-ts-{version}.tgz", "w:gz") as archive:
    info = tarfile.TarInfo("package/package.json")
    info.size = len(npm_package)
    archive.addfile(info, io.BytesIO(npm_package))

metadata = f"Metadata-Version: 2.1\nName: tabletheory-py\nVersion: {pep440}\n".encode("utf-8")
with zipfile.ZipFile(root / f"tabletheory_py-{pep440}-py3-none-any.whl", "w") as archive:
    archive.writestr(f"tabletheory_py-{pep440}.dist-info/METADATA", metadata)
    archive.writestr("tabletheory_py/version.json", version_json)

with tarfile.open(root / f"tabletheory_py-{pep440}.tar.gz", "w:gz") as archive:
    for name, payload in (
        (f"tabletheory_py-{pep440}/PKG-INFO", metadata),
        (f"tabletheory_py-{pep440}/src/tabletheory_py/version.json", version_json),
    ):
        info = tarfile.TarInfo(name)
        info.size = len(payload)
        archive.addfile(info, io.BytesIO(payload))
PY
}

extract_workflow_step_run() {
  local step_name="$1"
  local occurrence="${2:-1}"
  local workflow_path="${3:-${repo_root}/.github/workflows/release-hygiene.yml}"

  python3 - "${workflow_path}" "${step_name}" "${occurrence}" <<'PY'
import re
import sys

workflow_path, step_name, occurrence = sys.argv[1], sys.argv[2], int(sys.argv[3])
lines = open(workflow_path, "r", encoding="utf-8").read().splitlines()
seen = 0

for idx, line in enumerate(lines):
    if not re.match(r"\s*-\s+name:\s*" + re.escape(step_name) + r"\s*$", line):
        continue
    seen += 1
    if seen != occurrence:
        continue

    search_idx = idx + 1
    while search_idx < len(lines) and not re.match(r"\s*-\s+name:\s*", lines[search_idx]):
        run_match = re.match(r"(\s*)run:\s*\|\s*$", lines[search_idx])
        if not run_match:
            search_idx += 1
            continue

        run_indent = len(run_match.group(1))
        strip_indent = run_indent + 2
        block = []
        block_idx = search_idx + 1
        while block_idx < len(lines):
            block_line = lines[block_idx]
            if block_line.strip() == "":
                block.append("")
                block_idx += 1
                continue
            current_indent = len(block_line) - len(block_line.lstrip(" "))
            if current_indent <= run_indent:
                break
            if block_line.startswith(" " * strip_indent):
                block.append(block_line[strip_indent:])
            else:
                block.append(block_line.lstrip(" "))
            block_idx += 1

        print("\n".join(block))
        raise SystemExit(0)

        search_idx += 1

print(f"release-hygiene-policy-test: missing run block for workflow step {step_name}", file=sys.stderr)
raise SystemExit(1)
PY
}

# The retired trusted-base verifier-source selector and its capability-marker
# fixtures were removed with the single-source gate (operator ruling
# 2026-10-12). Their behavioral coverage now lives in the direct
# provenance/promotion-driver/release-cycle-state/release-baseline fixtures
# below, and in the single-source structural assertions at the end of this file.

common_args=(
  --repo "${repo}"
  --base premain
  --head staging
  --base-repo "${repo}"
  --head-repo "${repo}"
  --base-sha "${base_sha}"
  --head-sha "${head_sha}"
  --title "Promote staging to premain"
  --ref "refs/heads/premain=${base_sha}"
  --ref "refs/heads/staging=${head_sha}"
)

expect_success_contains \
  "release-lane-provenance: PASS" \
  bash "${checker}" "${common_args[@]}"

expect_failure_contains \
  "same-repository" \
  bash "${checker}" \
    --repo "${repo}" \
    --base premain \
    --head staging \
    --base-repo "${repo}" \
    --head-repo "attacker/TableTheory" \
    --base-sha "${base_sha}" \
    --head-sha "${head_sha}" \
    --title "Promote staging to premain" \
    --ref "refs/heads/premain=${base_sha}" \
    --ref "refs/heads/staging=${head_sha}"

expect_failure_contains \
  "head SHA" \
  bash "${checker}" \
    --repo "${repo}" \
    --base premain \
    --head staging \
    --base-repo "${repo}" \
    --head-repo "${repo}" \
    --base-sha "${base_sha}" \
    --head-sha "3333333333333333333333333333333333333333" \
    --title "Promote staging to premain" \
    --ref "refs/heads/premain=${base_sha}" \
    --ref "refs/heads/staging=${head_sha}"

expect_failure_contains \
  "base SHA" \
  bash "${checker}" \
    --repo "${repo}" \
    --base premain \
    --head staging \
    --base-repo "${repo}" \
    --head-repo "${repo}" \
    --base-sha "${base_sha}" \
    --head-sha "${head_sha}" \
    --title "Promote staging to premain" \
    --ref "refs/heads/premain=${advanced_base_sha}" \
    --ref "refs/heads/staging=${head_sha}"

expect_success_contains \
  "release-lane-provenance: PASS" \
  bash "${checker}" \
    --repo "${repo}" \
    --base premain \
    --head release-please--branches--premain \
    --base-repo "${repo}" \
    --head-repo "${repo}" \
    --base-sha "${base_sha}" \
    --head-sha "${head_sha}" \
    --title "chore(premain): release 1.9.3-rc" \
    --ref "refs/heads/premain=${base_sha}" \
    --ref "refs/heads/release-please--branches--premain=${head_sha}"

expect_failure_contains \
  "must advertise an RC version" \
  bash "${checker}" \
    --repo "${repo}" \
    --base premain \
    --head release-please--branches--premain \
    --base-repo "${repo}" \
    --head-repo "${repo}" \
    --base-sha "${base_sha}" \
    --head-sha "${head_sha}" \
    --title "chore(premain): release 1.9.3-beta.1" \
    --ref "refs/heads/premain=${base_sha}" \
    --ref "refs/heads/release-please--branches--premain=${head_sha}"

expect_failure_contains \
  "main PR must not advertise an RC version" \
  bash "${checker}" \
    --repo "${repo}" \
    --base main \
    --head release-please--branches--main \
    --base-repo "${repo}" \
    --head-repo "${repo}" \
    --base-sha "${base_sha}" \
    --head-sha "${head_sha}" \
    --title "chore(main): release 2.0.0-rc" \
    --ref "refs/heads/main=${base_sha}" \
    --ref "refs/heads/release-please--branches--main=${head_sha}"

expect_success_contains \
  "release-lane-provenance: PASS" \
  bash "${checker}" \
    --repo "${repo}" \
    --base main \
    --head release-please--branches--main \
    --base-repo "${repo}" \
    --head-repo "${repo}" \
    --base-sha "${base_sha}" \
    --head-sha "${head_sha}" \
    --title "chore(main): release 2.0.0" \
    --ref "refs/heads/main=${base_sha}" \
    --ref "refs/heads/release-please--branches--main=${head_sha}"

expect_failure_contains \
  "main PR must not advertise an RC version" \
  bash "${checker}" \
    --repo "${repo}" \
    --base main \
    --head premain \
    --base-repo "${repo}" \
    --head-repo "${repo}" \
    --base-sha "${base_sha}" \
    --head-sha "${head_sha}" \
    --title "Promote 2.0.0-rc.1 to main" \
    --ref "refs/heads/main=${base_sha}" \
    --ref "refs/heads/premain=${head_sha}"

expect_success_contains \
  "stale merged premain release-please RC PR" \
  bash "${checker}" \
    --repo "${repo}" \
    --base premain \
    --head release-please--branches--premain \
    --base-repo "${repo}" \
    --head-repo "${repo}" \
    --base-sha "${base_sha}" \
    --head-sha "${head_sha}" \
    --pr-state MERGED \
    --pr-merged true \
    --title "chore(premain): release 1.10.1-rc.1" \
    --ref "refs/heads/premain=${advanced_base_sha}" \
    --ref "refs/heads/release-please--branches--premain=${head_sha}"

expect_failure_contains \
  "base SHA" \
  bash "${checker}" \
    --repo "${repo}" \
    --base premain \
    --head release-please--branches--premain \
    --base-repo "${repo}" \
    --head-repo "${repo}" \
    --base-sha "${base_sha}" \
    --head-sha "${head_sha}" \
    --pr-state OPEN \
    --pr-merged false \
    --title "chore(premain): release 1.10.1-rc.1" \
    --ref "refs/heads/premain=${advanced_base_sha}" \
    --ref "refs/heads/release-please--branches--premain=${head_sha}"

# R1/5c4bce67 + R2/5d26307c: a same-repository PR that uses the retired reusable
# bootstrap prefix (with any content) must fail release-lane provenance. The
# prefix cannot launder a passing release check.
expect_failure_contains \
  "bootstrap branch prefixes are retired" \
  bash "${checker}" \
    --repo "${repo}" \
    --base main \
    --head "fix/release-hygiene-main-bootstrap-replace" \
    --base-repo "${repo}" \
    --head-repo "${repo}" \
    --base-sha "${base_sha}" \
    --head-sha "${head_sha}" \
    --title "replace allowlisted release scripts" \
    --ref "refs/heads/main=${base_sha}" \
    --ref "refs/heads/fix/release-hygiene-main-bootstrap-replace=${head_sha}"

expect_failure_contains \
  "bootstrap branch prefixes are retired" \
  bash "${checker}" \
    --repo "${repo}" \
    --base premain \
    --head "fix/release-hygiene-main-bootstrap-repair" \
    --base-repo "${repo}" \
    --head-repo "${repo}" \
    --base-sha "${base_sha}" \
    --head-sha "${head_sha}" \
    --title "bootstrap release hygiene repair" \
    --ref "refs/heads/premain=${base_sha}" \
    --ref "refs/heads/fix/release-hygiene-main-bootstrap-repair=${head_sha}"

expect_success_contains \
  "RC Release-As 1.9.3-rc" \
  bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
    --base premain \
    --head staging \
    --title "Promote staging to premain" \
    --body "Release-As: 1.9.3-rc" \
    --dry-run

expect_success_contains \
  "RC Release-As 1.9.3-rc.1" \
  bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
    --base premain \
    --head staging \
    --title "Promote staging to premain" \
    --body "Release-As: 1.9.3-rc.1" \
    --dry-run

expect_failure_contains \
  "X.Y.Z-rc or X.Y.Z-rc.N" \
  bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
    --base premain \
    --head staging \
    --title "Promote staging to premain" \
    --body "Release-As: 1.9.3" \
    --dry-run

expect_success_contains \
  "superseded older Release-As 3.0.5" \
  bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
    --base premain \
    --head staging \
    --title "Promote staging to premain" \
    --commit-message $'fix: mistaken stable footer\n\nRelease-As: 3.0.5' \
    --commit-message $'fix: correct RC footer\n\nRelease-As: 3.0.5-rc.1' \
    --commit-message "fix: later release-eligible repair" \
    --dry-run

expect_failure_contains \
  "X.Y.Z-rc or X.Y.Z-rc.N" \
  bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
    --base premain \
    --head staging \
    --title "Promote staging to premain" \
    --commit-message $'fix: correct older RC footer\n\nRelease-As: 3.0.5-rc.1' \
    --commit-message $'fix: newest footer is stable-shaped\n\nRelease-As: 3.0.5' \
    --dry-run

expect_success_contains \
  "generated premain RC PR" \
  bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
    --base premain \
    --head release-please--branches--premain \
    --title "chore(premain): release 1.9.3-rc" \
    --dry-run

expect_failure_contains \
  "must be RC-shaped" \
  bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
    --base premain \
    --head release-please--branches--premain \
    --title "chore(premain): release 1.9.3-beta.1" \
    --dry-run

watch_rc_title_regex='^chore\(premain\): release [0-9]+\.[0-9]+\.[0-9]+-rc(?:\.[0-9]+)?$'
if [[ "$(grep -Fc -- '-rc(?:\\.[0-9]+)?$' "${repo_root}/scripts/watch-release-cycle.sh")" -ne 2 ]]; then
  echo "release-hygiene-policy-test: watch-release-cycle must use first-or-numbered RC title matching"
  exit 1
fi

for title in \
  "chore(premain): release 3.0.4-rc" \
  "chore(premain): release 3.0.4-rc.1"; do
  jq -ne --arg title "${title}" --arg regex "${watch_rc_title_regex}" \
    '$title | test($regex)' >/dev/null || {
    echo "release-hygiene-policy-test: watch-release-cycle must accept RC title ${title}"
    exit 1
  }
done

if jq -ne --arg title "chore(premain): release 3.0.4-beta.1" --arg regex "${watch_rc_title_regex}" \
  '$title | test($regex)' >/dev/null; then
  echo "release-hygiene-policy-test: watch-release-cycle must reject non-RC prerelease titles"
  exit 1
fi

pending_fixture="$(mktemp -d)"
tmpdirs+=("${pending_fixture}")
mkdir -p \
  "${pending_fixture}/.github/workflows" \
  "${pending_fixture}/scripts" \
  "${pending_fixture}/ts" \
  "${pending_fixture}/py/src/tabletheory_py"
cat >"${pending_fixture}/.release-please-manifest.json" <<'JSON'
{".":"1.10.1-rc.1"}
JSON
cat >"${pending_fixture}/ts/package.json" <<'JSON'
{"version":"1.10.0"}
JSON
cat >"${pending_fixture}/ts/package-lock.json" <<'JSON'
{"version":"1.10.0","packages":{"":{"version":"1.10.0"}}}
JSON
cat >"${pending_fixture}/py/src/tabletheory_py/version.json" <<'JSON'
{"version":"1.10.0"}
JSON
touch \
  "${pending_fixture}/.github/workflows/release-hygiene.yml" \
  "${pending_fixture}/scripts/prepare-release-package-versions.py" \
  "${pending_fixture}/scripts/verify-release-package-version-assets.py" \
  "${pending_fixture}/scripts/watch-release-cycle.sh"
cp "${repo_root}/scripts/verify-go-semantic-import-version.sh" \
  "${pending_fixture}/scripts/verify-go-semantic-import-version.sh"
cat >"${pending_fixture}/go.mod" <<'EOF_GO_MOD'
module github.com/theory-cloud/tabletheory

go 1.26
EOF_GO_MOD

run_in_pending_fixture() (
  cd "${pending_fixture}"
  "$@"
)

expect_success_contains \
  "rc=1.10.1-rc.1" \
  env \
    GITHUB_BASE_REF=main \
    GITHUB_HEAD_REF=premain \
    RELEASE_CYCLE_ALLOW_PENDING_STABLE_PROMOTION=true \
    RELEASE_CYCLE_REPO_ROOT="${pending_fixture}" \
    bash "${repo_root}/scripts/verify-release-cycle-state.sh"

expect_failure_contains \
  "must be an absolute path" \
  env \
    RELEASE_CYCLE_REPO_ROOT=relative-target \
    bash "${repo_root}/scripts/verify-release-cycle-state.sh"

expect_success_contains \
  "premain -> main pending stable promotion 1.10.1-rc.1 -> 1.10.1" \
  run_in_pending_fixture \
    bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
      --base main \
      --head premain \
      --title "Promote premain to main" \
      --body "Release-As: 1.10.1" \
      --commit-message $'fix(security): recover release cycle\n\nRelease-As: 1.10.1-rc.1' \
      --published-baseline 1.10.0 \
      --dry-run

expect_failure_contains \
  "premain -> main Release-As footers must be stable X.Y.Z" \
  run_in_pending_fixture \
    bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
      --base main \
      --head premain \
      --title "Promote premain to main" \
      --body "Release-As: 1.10.1-rc.1" \
      --dry-run

expect_failure_contains \
  "premain -> main PR title must not be RC-shaped" \
  run_in_pending_fixture \
    bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
      --base main \
      --head premain \
      --title "Promote 1.10.1-rc.1 to main" \
      --body "Release-As: 1.10.1" \
      --dry-run

expect_failure_contains \
  "pending RC base 1.10.1" \
  run_in_pending_fixture \
    bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
      --base main \
      --head premain \
      --title "Promote premain to main" \
      --body "Release-As: 1.10.2" \
      --dry-run

expect_success_contains \
  "manifest-derived stable Release-As" \
  run_in_pending_fixture \
    bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
      --base main \
      --head premain \
      --title "Promote premain to main" \
      --body "" \
      --published-baseline 1.10.0 \
      --dry-run

# TTSEC2-M3-T3: every next-version candidate must be strictly greater than the
# highest published release, whatever its origin. These exercise the real guard:
# the manifest origin and the Release-As footer origin must both fail on an equal
# or lower candidate, an explicit empty baseline means "nothing published yet"
# (bootstrap) rather than a skipped check, and an absent baseline fails closed on
# the premain -> main path.
expect_failure_contains \
  "manifest candidate 1.10.1-rc.1 must be strictly greater than the highest published release 1.10.1" \
  run_in_pending_fixture \
    bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
      --base main \
      --head premain \
      --title "Promote premain to main" \
      --body "Release-As: 1.10.1" \
      --published-baseline 1.10.1 \
      --dry-run

expect_failure_contains \
  "must be strictly greater than the highest published release 1.10.2" \
  run_in_pending_fixture \
    bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
      --base main \
      --head premain \
      --title "Promote premain to main" \
      --body "Release-As: 1.10.1" \
      --published-baseline 1.10.2 \
      --dry-run

expect_failure_contains \
  "requires the highest published release baseline" \
  run_in_pending_fixture \
    bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
      --base main \
      --head premain \
      --title "Promote premain to main" \
      --body "Release-As: 1.10.1" \
      --dry-run

expect_success_contains \
  "manifest-derived stable Release-As" \
  run_in_pending_fixture \
    bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
      --base main \
      --head premain \
      --title "Promote premain to main" \
      --body "" \
      --published-baseline "" \
      --dry-run

expect_failure_contains \
  "staging -> premain Release-As footer candidate 1.10.1-rc.1 must be strictly greater than the highest published release 1.10.1" \
  bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
    --base premain \
    --head staging \
    --title "Promote staging to premain" \
    --body "Release-As: 1.10.1-rc.1" \
    --published-baseline 1.10.1 \
    --dry-run

expect_success_contains \
  "effective RC Release-As 1.10.2-rc.1" \
  bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
    --base premain \
    --head staging \
    --title "Promote staging to premain" \
    --body "Release-As: 1.10.2-rc.1" \
    --published-baseline 1.10.1 \
    --dry-run

stable_manifest_fixture="$(mktemp -d)"
tmpdirs+=("${stable_manifest_fixture}")
cp -R "${pending_fixture}/." "${stable_manifest_fixture}/"
cat >"${stable_manifest_fixture}/.release-please-manifest.json" <<'JSON'
{".":"1.10.1"}
JSON

run_in_stable_manifest_fixture() (
  cd "${stable_manifest_fixture}"
  "$@"
)

expect_failure_contains \
  "must carry a single-manifest RC" \
  run_in_stable_manifest_fixture \
    bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
      --base main \
      --head premain \
      --title "Promote premain to main" \
      --body "" \
      --dry-run

prerelease_fixture="$(mktemp -d)"
tmpdirs+=("${prerelease_fixture}")
write_prerelease_postcondition_fixture "${prerelease_fixture}" "2.0.0-rc"
expect_success_contains \
  "manifest 2.0.0-rc" \
  run_prerelease_postcondition_fixture "${prerelease_fixture}" \
    --expected-version 2.0.0-rc

prerelease_numbered_fixture="$(mktemp -d)"
tmpdirs+=("${prerelease_numbered_fixture}")
write_prerelease_postcondition_fixture "${prerelease_numbered_fixture}" "2.0.0-rc.1"
expect_success_contains \
  "manifest 2.0.0-rc.1" \
  run_prerelease_postcondition_fixture "${prerelease_numbered_fixture}" \
    --expected-version 2.0.0-rc.1

prerelease_bad_title_fixture="$(mktemp -d)"
tmpdirs+=("${prerelease_bad_title_fixture}")
write_prerelease_postcondition_fixture \
  "${prerelease_bad_title_fixture}" \
  "2.0.0-beta.1" \
  "chore(premain): release 2.0.0-beta.1"
expect_failure_contains \
  "not RC-shaped" \
  run_prerelease_postcondition_fixture "${prerelease_bad_title_fixture}"

prerelease_missing_manifest_fixture="$(mktemp -d)"
tmpdirs+=("${prerelease_missing_manifest_fixture}")
write_prerelease_postcondition_fixture \
  "${prerelease_missing_manifest_fixture}" \
  "2.0.0-rc" \
  "chore(premain): release 2.0.0-rc" \
  false \
  true
expect_failure_contains \
  "missing single-manifest prerelease files" \
  run_prerelease_postcondition_fixture "${prerelease_missing_manifest_fixture}"

prerelease_missing_changelog_fixture="$(mktemp -d)"
tmpdirs+=("${prerelease_missing_changelog_fixture}")
write_prerelease_postcondition_fixture \
  "${prerelease_missing_changelog_fixture}" \
  "2.0.0-rc" \
  "chore(premain): release 2.0.0-rc" \
  true \
  false
expect_failure_contains \
  "missing single-manifest prerelease files" \
  run_prerelease_postcondition_fixture "${prerelease_missing_changelog_fixture}"

prerelease_non_rc_manifest_fixture="$(mktemp -d)"
tmpdirs+=("${prerelease_non_rc_manifest_fixture}")
write_prerelease_postcondition_fixture \
  "${prerelease_non_rc_manifest_fixture}" \
  "2.0.0-rc" \
  "chore(premain): release 2.0.0-rc" \
  true \
  true \
  "2.0.0-beta.1"
expect_failure_contains \
  "manifest version is not RC-shaped" \
  run_prerelease_postcondition_fixture "${prerelease_non_rc_manifest_fixture}"

expect_success_contains \
  "published v1.9.3-rc" \
  bash "${repo_root}/scripts/verify-release-created-postcondition.sh" \
    --kind prerelease \
    --branch premain \
    --release-created true \
    --tag-name v1.9.3-rc \
    --commit-message "chore(premain): release 1.9.3-rc"

expect_success_contains \
  "published v1.9.3-rc.1" \
  bash "${repo_root}/scripts/verify-release-created-postcondition.sh" \
    --kind prerelease \
    --branch premain \
    --release-created true \
    --tag-name v1.9.3-rc.1 \
    --commit-message "chore(premain): release 1.9.3-rc.1"

expect_failure_contains \
  "X.Y.Z-rc or X.Y.Z-rc.N" \
  bash "${repo_root}/scripts/verify-release-created-postcondition.sh" \
    --kind prerelease \
    --branch premain \
    --release-created true \
    --tag-name v1.9.3 \
    --commit-message "chore(premain): release 1.9.3-rc"

expect_failure_contains \
  "X.Y.Z-rc or X.Y.Z-rc.N" \
  bash "${repo_root}/scripts/verify-release-created-postcondition.sh" \
    --kind prerelease \
    --branch premain \
    --release-created true \
    --tag-name v1.9.3-beta.1 \
    --commit-message "chore(premain): release 1.9.3-rc"

expect_failure_contains \
  "X.Y.Z-rc or X.Y.Z-rc.N" \
  bash "${repo_root}/scripts/verify-release-created-postcondition.sh" \
    --kind prerelease \
    --branch premain \
    --release-created true \
    --tag-name v1.9.3-rc. \
    --commit-message "chore(premain): release 1.9.3-rc"

expect_failure_contains \
  "does not match generated RC release 1.9.3-rc" \
  bash "${repo_root}/scripts/verify-release-created-postcondition.sh" \
    --kind prerelease \
    --branch premain \
    --release-created true \
    --tag-name v1.9.3-rc.1 \
    --commit-message "chore(premain): release 1.9.3-rc"

expect_failure_contains \
  "non-generated RC release PR merge" \
  bash "${repo_root}/scripts/verify-release-created-postcondition.sh" \
    --kind prerelease \
    --branch premain \
    --release-created true \
    --tag-name v1.9.3-rc \
    --commit-message "chore(premain): release 1.9.3-rc."

for version in 2.0.0-rc 2.0.0-rc.1; do
  package_fixture="$(mktemp -d)"
  tmpdirs+=("${package_fixture}")
  write_package_version_fixture "${package_fixture}"
  expect_success_contains \
    "version=${version}" \
    python3 "${repo_root}/scripts/prepare-release-package-versions.py" \
      --tag-name "v${version}" \
      --repo-root "${package_fixture}"
  assert_package_version_fixture "${package_fixture}" "${version}"

  assets_fixture="$(mktemp -d)"
  tmpdirs+=("${assets_fixture}")
  write_release_asset_fixture "${assets_fixture}" "${version}"
  expect_success_contains \
    "version=${version}" \
    python3 "${repo_root}/scripts/verify-release-package-version-assets.py" \
      --tag-name "v${version}" \
      --assets-dir "${assets_fixture}"
done

expect_failure_contains \
  "X.Y.Z-rc or X.Y.Z-rc.N" \
  python3 "${repo_root}/scripts/prepare-release-package-versions.py" \
    --version 2.0.0-beta.1 \
    --repo-root "${repo_root}"

expect_failure_contains \
  "X.Y.Z-rc or X.Y.Z-rc.N" \
  python3 "${repo_root}/scripts/verify-release-package-version-assets.py" \
    --version 2.0.0-rc. \
    --assets-dir "${repo_root}/release-assets"

mismatch_assets_fixture="$(mktemp -d)"
tmpdirs+=("${mismatch_assets_fixture}")
write_release_asset_fixture "${mismatch_assets_fixture}" "2.0.0-rc.1"
expect_failure_contains \
  "!= '2.0.0-rc'" \
  python3 "${repo_root}/scripts/verify-release-package-version-assets.py" \
    --tag-name v2.0.0-rc \
    --assets-dir "${mismatch_assets_fixture}"

stable_pr_fixture="$(mktemp -d)"
tmpdirs+=("${stable_pr_fixture}")
git -C "${stable_pr_fixture}" init -q
git -C "${stable_pr_fixture}" config user.email fixture@example.com
git -C "${stable_pr_fixture}" config user.name "Release Fixture"
cat >"${stable_pr_fixture}/.release-please-manifest.json" <<'JSON'
{
  ".": "2.0.1-rc.1"
}
JSON
cat >"${stable_pr_fixture}/CHANGELOG.md" <<'MD'
# Changelog

## Unreleased

### Bug Fixes

* future work

## [2.0.1-rc.1](https://github.com/theory-cloud/TableTheory/compare/v2.0.1-rc...v2.0.1-rc.1) (2026-07-07)


### Bug Fixes

* release lane repair
MD
git -C "${stable_pr_fixture}" add .
git -C "${stable_pr_fixture}" commit -q -m "fixture pending stable"

expect_success_contains \
  "stable-release-pr: dry-run generated chore(main): release 2.0.1" \
  env RELEASE_DATE=2026-07-07 \
    python3 "${repo_root}/scripts/create-stable-release-pr.py" \
      --repo theory-cloud/TableTheory \
      --base main \
      --head release-please--branches--main \
      --expected-version 2.0.1 \
      --repo-root "${stable_pr_fixture}" \
      --dry-run

# TTSEC2-M3-T3 computation origin: the version release-pr.yml computes from the
# single manifest and hands to the stable Release PR generator must also be
# strictly greater than the highest published release. The guard runs before any
# repository mutation, so an equal or lower computation fails fast.
expect_failure_contains \
  "computed stable version 2.0.1 must be strictly greater than the highest published release 2.0.1" \
  python3 "${repo_root}/scripts/create-stable-release-pr.py" \
    --repo theory-cloud/TableTheory \
    --base main \
    --head release-please--branches--main \
    --expected-version 2.0.1 \
    --repo-root "${stable_pr_fixture}" \
    --published-baseline 2.0.1 \
    --dry-run

expect_failure_contains \
  "must be strictly greater than the highest published release 2.0.2" \
  python3 "${repo_root}/scripts/create-stable-release-pr.py" \
    --repo theory-cloud/TableTheory \
    --base main \
    --head release-please--branches--main \
    --expected-version 2.0.1 \
    --repo-root "${stable_pr_fixture}" \
    --published-baseline 2.0.2 \
    --dry-run

monotonic_fixture="$(mktemp -d)"
tmpdirs+=("${monotonic_fixture}")
git -C "${monotonic_fixture}" init -q
git -C "${monotonic_fixture}" config user.email fixture@example.com
git -C "${monotonic_fixture}" config user.name "Release Fixture"
cat >"${monotonic_fixture}/.release-please-manifest.json" <<'JSON'
{
  ".": "2.0.2-rc.1"
}
JSON
cat >"${monotonic_fixture}/CHANGELOG.md" <<'MD'
# Changelog

## Unreleased

## [2.0.2-rc.1](https://github.com/theory-cloud/TableTheory/compare/v2.0.2-rc...v2.0.2-rc.1) (2026-07-07)


### Bug Fixes

* release lane repair
MD
git -C "${monotonic_fixture}" add .
git -C "${monotonic_fixture}" commit -q -m "fixture monotonic"

expect_success_contains \
  "dry-run generated chore(main): release 2.0.2" \
  env RELEASE_DATE=2026-07-07 \
    python3 "${repo_root}/scripts/create-stable-release-pr.py" \
      --repo theory-cloud/TableTheory \
      --base main \
      --head release-please--branches--main \
      --expected-version 2.0.2 \
      --repo-root "${monotonic_fixture}" \
      --published-baseline 2.0.1 \
      --dry-run

grep -Fq '".": "2.0.1"' "${stable_pr_fixture}/.release-please-manifest.json" || {
  echo "release-hygiene-policy-test: stable Release PR generator must normalize the manifest to stable"
  exit 1
}

grep -Fq '## [2.0.1](https://github.com/theory-cloud/TableTheory/compare/v2.0.1-rc.1...v2.0.1) (2026-07-07)' "${stable_pr_fixture}/CHANGELOG.md" || {
  echo "release-hygiene-policy-test: stable Release PR generator must add the stable changelog section"
  exit 1
}

grep -Fq 'Stable promotion of `v2.0.1-rc.1`.' "${stable_pr_fixture}/CHANGELOG.md" || {
  echo "release-hygiene-policy-test: stable Release PR generator must record stable promotion notes"
  exit 1
}

if [[ "$(git -C "${stable_pr_fixture}" log -1 --format=%s)" != "chore(main): release 2.0.1" ]]; then
  echo "release-hygiene-policy-test: stable Release PR generator must use the stable release commit subject"
  exit 1
fi

python3 - "${repo_root}/scripts/create-stable-release-pr.py" <<'PY'
import importlib.util
import sys
from pathlib import Path

module_path = Path(sys.argv[1])
spec = importlib.util.spec_from_file_location("stable_release_pr", module_path)
module = importlib.util.module_from_spec(spec)
assert spec.loader is not None
spec.loader.exec_module(module)

body = module.stable_pull_request_body(
    "theory-cloud/TableTheory",
    "2.0.1-rc.1",
    "2.0.1",
    "2026-07-07",
)

if module.PENDING_RELEASE_LABEL != "autorelease: pending":
    raise SystemExit("stable Release PR generator must use the Release Please pending label")
source = module_path.read_text(encoding="utf-8")
for required in (
    "createCommitOnBranch",
    "expectedHeadOid",
    "GitHub did not report a verified signature",
    "create_or_reset_branch",
):
    if required not in source:
        raise SystemExit(f"stable Release PR generator must contain {required}")
if "\n---\n\n\n## [2.0.1]" not in body:
    raise SystemExit("stable Release PR body must expose stable release notes between Release Please delimiters")
if "## [2.0.1-rc" in body:
    raise SystemExit("stable Release PR body must not advertise an RC release")
PY

repair_fixture="$(mktemp -d)"
tmpdirs+=("${repair_fixture}")
git init --bare "${repair_fixture}/remote.git" -q
git -C "${repair_fixture}" init -q work
git -C "${repair_fixture}/work" config user.email fixture@example.com
git -C "${repair_fixture}/work" config user.name "Release Fixture"
git -C "${repair_fixture}/work" remote add origin "${repair_fixture}/remote.git"
mkdir -p "${repair_fixture}/work/scripts" "${repair_fixture}/work/.github/workflows"
cat >"${repair_fixture}/work/.release-please-manifest.json" <<'JSON'
{
  ".": "2.0.1-rc.1"
}
JSON
cat >"${repair_fixture}/work/scripts/create-stable-release-pr.py" <<'PY'
PENDING_RELEASE_LABEL = "autorelease: pending"
PY
cat >"${repair_fixture}/work/.github/workflows/release-pr.yml" <<'YAML'
steps:
  - run: python3 scripts/create-stable-release-pr.py
YAML
git -C "${repair_fixture}/work" add .
git -C "${repair_fixture}/work" commit -q -m "pending stable fixture"
protected_ref="refs/heads/ma""in"
git -C "${repair_fixture}/work" push -q origin "HEAD:${protected_ref}"
git -C "${repair_fixture}/work" switch -q -c premain

expect_success_contains \
  "premain-pending-stable-repair: PASS (premain=2.0.1-rc.1, origin/main=2.0.1-rc.1, stable=2.0.1)" \
  env GITHUB_REF_NAME=premain \
    bash "${repo_root}/scripts/verify-premain-pending-stable-repair.sh" \
      --repo-root "${repair_fixture}/work"

python3 - "${repair_fixture}/work/.release-please-manifest.json" <<'PY'
import json
import sys
from pathlib import Path

path = Path(sys.argv[1])
path.write_text(json.dumps({".": "2.0.1-rc.2"}, indent=2) + "\n", encoding="utf-8")
PY

expect_failure_contains \
  "checked-out premain RC 2.0.1-rc.2 does not match origin/main pending RC 2.0.1-rc.1" \
  env GITHUB_REF_NAME=premain \
    bash "${repo_root}/scripts/verify-premain-pending-stable-repair.sh" \
      --repo-root "${repair_fixture}/work"

if grep -Fq "secrets.RELEASE_PLEASE_TOKEN" "${repo_root}/.github/workflows/release-hygiene.yml"; then
  echo "release-hygiene-policy-test: release hygiene must not expose release-please token"
  exit 1
fi

grep -Fq "persist-credentials: false" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: release hygiene checkouts must disable persisted credentials"
  exit 1
}

if grep -Fq "merge_group:" "${repo_root}/.github/workflows/quality-gates.yml"; then
  echo "release-hygiene-policy-test: quality gates must not use the prohibited merge-queue event"
  exit 1
fi

grep -Fq "fetch-depth: 0" "${repo_root}/.github/workflows/quality-gates.yml" || {
  echo "release-hygiene-policy-test: quality gates must fetch full history for in-flight premain RC repair verification"
  exit 1
}

grep -Fq "+refs/heads/premain:refs/remotes/origin/premain" "${repo_root}/.github/workflows/quality-gates.yml" || {
  echo "release-hygiene-policy-test: quality gates must fetch origin/premain for in-flight premain RC repair verification"
  exit 1
}

grep -Fq "+refs/heads/staging:refs/remotes/origin/staging" "${repo_root}/.github/workflows/quality-gates.yml" || {
  echo "release-hygiene-policy-test: quality gates must fetch origin/staging for in-flight premain RC repair verification"
  exit 1
}

if grep -Fq "merge_group:" "${repo_root}/.github/workflows/release-hygiene.yml"; then
  echo "release-hygiene-policy-test: release hygiene must not use the prohibited merge-queue event"
  exit 1
fi

for workflow in \
  "${repo_root}/.github/workflows/prerelease.yml" \
  "${repo_root}/.github/workflows/prerelease-pr.yml"; do
  grep -Fq "scripts/verify-premain-pending-stable-repair.sh" "${workflow}" || {
    echo "release-hygiene-policy-test: ${workflow} must classify pending stable repair before release-please"
    exit 1
  }
  grep -Fq "pending_stable_repair != 'true'" "${workflow}" || {
    echo "release-hygiene-policy-test: ${workflow} must skip prerelease work during pending stable repair"
    exit 1
  }
done

for workflow in \
  "${repo_root}/.github/workflows/typescript.yml" \
  "${repo_root}/.github/workflows/python.yml" \
  "${repo_root}/.github/workflows/unit-cover.yml"; do
  grep -Fq "paths-ignore:" "${workflow}" || {
    echo "release-hygiene-policy-test: ${workflow} must use trigger-level paths-ignore for release-please PRs"
    exit 1
  }
  grep -Fq '".release-please-manifest.json"' "${workflow}" || {
    echo "release-hygiene-policy-test: ${workflow} must ignore manifest-only release-please PR changes"
    exit 1
  }
  grep -Fq '"CHANGELOG.md"' "${workflow}" || {
    echo "release-hygiene-policy-test: ${workflow} must ignore changelog-only release-please PR changes"
    exit 1
  }
  if grep -Eq '^[[:space:]]*paths:' "${workflow}"; then
    echo "release-hygiene-policy-test: ${workflow} must not replace normal PR validation with a paths allowlist"
    exit 1
  fi
done

if grep -Fq -- "--queue-freshness" "${repo_root}/.github/workflows/release-hygiene.yml"; then
  echo "release-hygiene-policy-test: release hygiene must not bypass strict live-ref freshness"
  exit 1
fi

if grep -Fq -- "--queue-freshness" "${repo_root}/scripts/verify-release-lane-provenance.sh"; then
  echo "release-hygiene-policy-test: provenance must not expose a merge-queue freshness bypass"
  exit 1
fi

grep -Fq 'bash scripts/verify-release-lane-provenance.sh "${provenance_args[@]}"' "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: release hygiene must run same-repository provenance from the single head checkout"
  exit 1
}

grep -Fq "does not match same-repository refs/heads" "${repo_root}/scripts/verify-release-lane-provenance.sh" || {
  echo "release-hygiene-policy-test: provenance must enforce exact live branch SHAs"
  exit 1
}

# Single-source gate (operator ruling 2026-10-12). Every check executes from ONE
# checkout of the promoted head; the retired verifier-source selector, its
# capability-marker negotiation, and the base-SHA second checkout are gone. Each
# trusted-base assertion below is replaced by an exact head invocation, a
# structural single-source prohibition, or a direct fixture against the real
# verifier that the selector used to feature-negotiate.
hygiene_workflow="${repo_root}/.github/workflows/release-hygiene.yml"

# Replaces the "Resolve release verifier source" presence check: there is no
# verifier source to resolve any more.
if grep -Fq "Resolve release verifier source" "${hygiene_workflow}"; then
  echo "release-hygiene-policy-test: the release verifier source selector must be removed"
  exit 1
fi

# Replaces the trusted-base capability-marker probes ("newest Release-As footer
# wins", "v2 single-manifest", "RC-first PR titles/versions", "release-please
# v5", "deterministic stable Release PR", "direct protected-PR", "Go semantic
# import", "pending-major-transition"). Each capability is now verified by the
# real verifier the workflow runs from the head checkout, and by the direct
# fixtures in this file, instead of by marker strings in a trusted base.
for marker_negotiation in \
  "release-verifier-source:" \
  "collect_v2_marker_reasons" \
  "newest Release-As footer wins" \
  "trusted base lacks v2 single-manifest" \
  "single release-please manifest" \
  "accept release-please first RC and numbered later RC PR titles" \
  "accept release-please first RC and numbered later RC version syntax" \
  "release-please v5" \
  "manifest-derived stable Release-As" \
  "resolved promotion-driver supply-chain marker" \
  "deterministic stable Release PR marker" \
  "direct protected-PR verifier marker" \
  "Go semantic import verifier marker" \
  "Go semantic import supply-chain marker" \
  "pending-major-transition"; do
  if grep -Fq -- "${marker_negotiation}" "${hygiene_workflow}"; then
    echo "release-hygiene-policy-test: release hygiene must not negotiate verifier capability markers (${marker_negotiation})"
    exit 1
  fi
done

# Replaces the VERIFIER_ROOT/VERIFIER_LABEL invocations and source-log pins: every
# verifier now runs from the single head checkout by plain relative path.
grep -Fq 'bash scripts/verify-release-cycle-state.sh' "${hygiene_workflow}" || {
  echo "release-hygiene-policy-test: release-cycle state must run from the single head checkout"
  exit 1
}

grep -Fq 'bash scripts/verify-branch-release-supply-chain.sh' "${hygiene_workflow}" || {
  echo "release-hygiene-policy-test: branch release supply-chain must run from the single head checkout"
  exit 1
}

grep -Fq 'bash scripts/verify-promotion-release-driver.sh' "${hygiene_workflow}" || {
  echo "release-hygiene-policy-test: promotion driver must run from the single head checkout"
  exit 1
}

grep -Fq 'bash scripts/verify-main-release-pr-postcondition.sh' "${hygiene_workflow}" || {
  echo "release-hygiene-policy-test: the main RC postcondition must run from the single head checkout"
  exit 1
}

# Replaces the trusted-base checkout pins: exactly one checkout, of the promoted
# head, without persisted credentials, and nothing from a second checkout.
checkout_count="$(grep -Fc 'uses: actions/checkout@' "${hygiene_workflow}")"
if [[ "${checkout_count}" -ne 1 ]]; then
  echo "release-hygiene-policy-test: release hygiene must perform exactly one checkout (found ${checkout_count})"
  exit 1
fi

grep -Fq "github.event.pull_request.head.sha" "${hygiene_workflow}" || {
  echo "release-hygiene-policy-test: the single checkout must use the exact PR head SHA"
  exit 1
}

grep -Fq "persist-credentials: false" "${hygiene_workflow}" || {
  echo "release-hygiene-policy-test: the single checkout must not persist credentials"
  exit 1
}

# Replaces the R3 selector/no-op/hostile-head probe fixtures: no second checkout,
# no verifier-source indirection, no PR-head-relative verifier path, and no
# fail-closed "base lacks markers" branch may exist anywhere in the workflow.
for single_source_violation in \
  "trusted-release" \
  "path: pr" \
  "VERIFIER_ROOT" \
  "VERIFIER_LABEL" \
  "protected-pr-head-v2" \
  "land the verifier update on" \
  'verifier_root="."' \
  "root=\${trusted_root}" \
  "label=trusted-base"; do
  if grep -Fq -- "${single_source_violation}" "${hygiene_workflow}"; then
    echo "release-hygiene-policy-test: release hygiene must be a single-source gate (${single_source_violation})"
    exit 1
  fi
done

# The workflow documents the ruled trust-model change in plain terms.
grep -Fq "Single-source gate (operator ruling 2026-10-12)" "${hygiene_workflow}" || {
  echo "release-hygiene-policy-test: release hygiene must document the single-source trust model"
  exit 1
}

grep -Fq "PR #681" "${hygiene_workflow}" || {
  echo "release-hygiene-policy-test: release hygiene must record why the trusted-base design was replaced"
  exit 1
}

# R1/5c4bce67 + R2/5d26307c: the reusable bootstrap allowlist and its branch-prefix
# exemption must be gone from the workflow entirely.
if grep -Fq "Verify release-hygiene bootstrap scope" "${repo_root}/.github/workflows/release-hygiene.yml"; then
  echo "release-hygiene-policy-test: the release-hygiene bootstrap allowlist must be removed"
  exit 1
fi

if grep -Fq "fix/release-hygiene-main-bootstrap-" "${repo_root}/.github/workflows/release-hygiene.yml"; then
  echo "release-hygiene-policy-test: no reusable bootstrap branch prefix may skip release-lane provenance"
  exit 1
fi

if grep -Fq "pulls/\${PR_NUMBER}/files" "${repo_root}/.github/workflows/release-hygiene.yml"; then
  echo "release-hygiene-policy-test: release hygiene must not reintroduce a changed-file bootstrap allowlist"
  exit 1
fi

grep -Fq "name: Verify release-lane same-repository provenance" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: release hygiene must always verify release-lane provenance"
  exit 1
}

# R2/5d26307c: the provenance guard itself must reject the retired bootstrap prefix.
grep -Fq "bootstrap branch prefixes are retired" "${repo_root}/scripts/verify-release-lane-provenance.sh" || {
  echo "release-hygiene-policy-test: provenance must reject the retired bootstrap branch prefix"
  exit 1
}

grep -Fq 'RELEASE_CYCLE_REPO_ROOT: ${{ github.workspace }}' "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: release cycle state must validate the single head checkout"
  exit 1
}

# TTSEC2-M3-T3 (F4): the released-baseline floor must be the semver MAXIMUM over
# published stable releases, not the newest-created release; drafts and
# prereleases must be excluded even when they are newer; the read must page past
# a full page; and any API, JSON, or semver fault must fail closed instead of
# yielding an unchecked or empty baseline. The real workflow steps run here
# against a stubbed `gh`, so no release, network, or cloud call happens.
write_release_baseline_fixture() {
  local root="$1"
  local mode="$2"

  mkdir -p "${root}"
  case "${mode}" in
    out-of-order)
      python3 - "${root}/page1.json" <<'PY'
import json
import sys

# created_at order is deliberately the reverse of semver order, and the
# higher draft/prerelease entries must not become the floor.
releases = [
    {"tag_name": "v1.9.3", "draft": False, "prerelease": False, "created_at": "2026-10-09T00:00:00Z"},
    {"tag_name": "v4.0.2", "draft": False, "prerelease": False, "created_at": "2026-01-02T00:00:00Z"},
    {"tag_name": "v9.9.9", "draft": True, "prerelease": False, "created_at": "2026-10-10T00:00:00Z"},
    {"tag_name": "v5.0.0-rc.1", "draft": False, "prerelease": True, "created_at": "2026-10-10T00:00:00Z"},
]
with open(sys.argv[1], "w", encoding="utf-8") as handle:
    json.dump(releases, handle)
PY
      ;;
    paginated)
      python3 - "${root}/page1.json" "${root}/page2.json" <<'PY'
import json
import sys

# A full first page forces the reader to ask for page 2, where the highest
# published stable release lives. v1.9.0 on page 1 also separates a numeric
# semver maximum from a textual one, which would pick 1.9.0 over 1.10.0.
page1 = [{"tag_name": f"v1.0.{i}", "draft": False, "prerelease": False} for i in range(98)]
page1.append({"tag_name": "v1.9.0", "draft": False, "prerelease": False})
page1.append({"tag_name": "v1.0.99-rc.1", "draft": False, "prerelease": True})
page2 = [{"tag_name": "v1.10.0", "draft": False, "prerelease": False}]
with open(sys.argv[1], "w", encoding="utf-8") as handle:
    json.dump(page1, handle)
with open(sys.argv[2], "w", encoding="utf-8") as handle:
    json.dump(page2, handle)
PY
      ;;
    bootstrap-empty)
      printf '[]\n' >"${root}/page1.json"
      ;;
    only-unpublished)
      python3 - "${root}/page1.json" <<'PY'
import json
import sys

releases = [
    {"tag_name": "v2.0.0", "draft": True, "prerelease": False},
    {"tag_name": "v1.9.3-rc.1", "draft": False, "prerelease": True},
]
with open(sys.argv[1], "w", encoding="utf-8") as handle:
    json.dump(releases, handle)
PY
      ;;
    non-semver)
      printf '[{"tag_name":"nightly","draft":false,"prerelease":false}]\n' >"${root}/page1.json"
      ;;
    *)
      echo "release-hygiene-policy-test: unknown release baseline fixture ${mode}" >&2
      exit 1
      ;;
  esac
}

# run_release_baseline_step SCRIPT FIXTURE MODE OUT_FILE GH_OUTPUT -> inner exit status
run_release_baseline_step() {
  local step_script="$1"
  local fixture="$2"
  local mode="$3"
  local out_file="$4"
  local gh_output="$5"

  local fail="" invalid=""
  case "${mode}" in
    ok) ;;
    api-fail) fail=1 ;;
    invalid-json) invalid=1 ;;
    *) echo "release-hygiene-policy-test: unknown release baseline mode ${mode}" >&2; exit 1 ;;
  esac

  local status
  set +e
  env \
    PATH="${fake_gh_bin}:${PATH}" \
    GITHUB_REPOSITORY="${repo}" \
    GITHUB_OUTPUT="${gh_output}" \
    FAKE_GH_DIR="${fixture}" \
    FAKE_GH_CALLS="${fixture}/calls" \
    FAKE_GH_FAIL="${fail}" \
    FAKE_GH_INVALID_JSON="${invalid}" \
    bash "${step_script}" >"${out_file}" 2>&1
  status=$?
  set -e
  printf '%s' "${status}"
}

assert_release_baseline() {
  local label="$1"
  local status="$2"
  local out_file="$3"
  local gh_output="$4"
  local expected="$5"

  if [[ "${status}" -ne 0 ]]; then
    cat "${out_file}"
    echo "release-hygiene-policy-test: ${label}: expected the released-baseline step to succeed"
    exit 1
  fi
  if ! grep -Fxq "baseline=${expected}" "${gh_output}"; then
    cat "${out_file}"
    cat "${gh_output}"
    echo "release-hygiene-policy-test: ${label}: expected baseline=${expected}"
    exit 1
  fi
}

assert_release_baseline_failure() {
  local label="$1"
  local status="$2"
  local out_file="$3"
  local gh_output="$4"
  local needle="$5"

  if [[ "${status}" -eq 0 ]]; then
    cat "${out_file}"
    echo "release-hygiene-policy-test: ${label}: expected the released-baseline step to fail closed"
    exit 1
  fi
  if [[ -n "${needle}" ]] && ! grep -Fq -- "${needle}" "${out_file}"; then
    cat "${out_file}"
    echo "release-hygiene-policy-test: ${label}: expected failure to mention: ${needle}"
    exit 1
  fi
  if grep -q '^baseline=' "${gh_output}"; then
    cat "${gh_output}"
    echo "release-hygiene-policy-test: ${label}: a failed baseline read must not emit a baseline"
    exit 1
  fi
}

fake_gh_bin="$(mktemp -d)"
tmpdirs+=("${fake_gh_bin}")
cat >"${fake_gh_bin}/gh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" != "api" ]]; then
  echo "fake-gh: unexpected invocation: $*" >&2
  exit 64
fi
if [[ -n "${FAKE_GH_FAIL:-}" ]]; then
  echo "fake-gh: HTTP 502 Bad Gateway" >&2
  exit 1
fi
page="$(sed -n 's/.*page=\([0-9][0-9]*\).*/\1/p' <<<"${2:-}")"
page="${page:-1}"
printf '%s\n' "${page}" >>"${FAKE_GH_CALLS}"
if [[ -n "${FAKE_GH_INVALID_JSON:-}" ]]; then
  printf 'this is not json\n'
  exit 0
fi
if [[ -f "${FAKE_GH_DIR}/page${page}.json" ]]; then
  cat "${FAKE_GH_DIR}/page${page}.json"
else
  printf '[]\n'
fi
SH
chmod +x "${fake_gh_bin}/gh"

hygiene_baseline_step="$(mktemp)"
tmpdirs+=("${hygiene_baseline_step}")
extract_workflow_step_run "Resolve highest published release" 1 \
  "${repo_root}/.github/workflows/release-hygiene.yml" >"${hygiene_baseline_step}"

release_pr_baseline_step="$(mktemp)"
tmpdirs+=("${release_pr_baseline_step}")
extract_workflow_step_run "Resolve highest published release" 1 \
  "${repo_root}/.github/workflows/release-pr.yml" >"${release_pr_baseline_step}"

for baseline_step in "${hygiene_baseline_step}" "${release_pr_baseline_step}"; do
  baseline_label="$(basename "${baseline_step}")"

  baseline_fixture="$(mktemp -d)"
  tmpdirs+=("${baseline_fixture}")
  write_release_baseline_fixture "${baseline_fixture}" out-of-order
  baseline_out="$(mktemp)"
  tmpdirs+=("${baseline_out}")
  baseline_gh_out="$(mktemp)"
  tmpdirs+=("${baseline_gh_out}")
  assert_release_baseline \
    "${baseline_label} out-of-order stables" \
    "$(run_release_baseline_step "${baseline_step}" "${baseline_fixture}" ok "${baseline_out}" "${baseline_gh_out}")" \
    "${baseline_out}" "${baseline_gh_out}" "4.0.2"

  baseline_fixture="$(mktemp -d)"
  tmpdirs+=("${baseline_fixture}")
  write_release_baseline_fixture "${baseline_fixture}" paginated
  baseline_out="$(mktemp)"
  tmpdirs+=("${baseline_out}")
  baseline_gh_out="$(mktemp)"
  tmpdirs+=("${baseline_gh_out}")
  assert_release_baseline \
    "${baseline_label} paginated list" \
    "$(run_release_baseline_step "${baseline_step}" "${baseline_fixture}" ok "${baseline_out}" "${baseline_gh_out}")" \
    "${baseline_out}" "${baseline_gh_out}" "1.10.0"
  if [[ "$(tr -d '\n' <"${baseline_fixture}/calls")" != "12" ]]; then
    cat "${baseline_fixture}/calls"
    echo "release-hygiene-policy-test: ${baseline_label}: a full page must be followed by a second page request"
    exit 1
  fi

  baseline_fixture="$(mktemp -d)"
  tmpdirs+=("${baseline_fixture}")
  write_release_baseline_fixture "${baseline_fixture}" bootstrap-empty
  baseline_out="$(mktemp)"
  tmpdirs+=("${baseline_out}")
  baseline_gh_out="$(mktemp)"
  tmpdirs+=("${baseline_gh_out}")
  assert_release_baseline \
    "${baseline_label} bootstrap empty" \
    "$(run_release_baseline_step "${baseline_step}" "${baseline_fixture}" ok "${baseline_out}" "${baseline_gh_out}")" \
    "${baseline_out}" "${baseline_gh_out}" ""

  baseline_fixture="$(mktemp -d)"
  tmpdirs+=("${baseline_fixture}")
  write_release_baseline_fixture "${baseline_fixture}" only-unpublished
  baseline_out="$(mktemp)"
  tmpdirs+=("${baseline_out}")
  baseline_gh_out="$(mktemp)"
  tmpdirs+=("${baseline_gh_out}")
  assert_release_baseline \
    "${baseline_label} drafts and prereleases only" \
    "$(run_release_baseline_step "${baseline_step}" "${baseline_fixture}" ok "${baseline_out}" "${baseline_gh_out}")" \
    "${baseline_out}" "${baseline_gh_out}" ""

  baseline_fixture="$(mktemp -d)"
  tmpdirs+=("${baseline_fixture}")
  write_release_baseline_fixture "${baseline_fixture}" out-of-order
  baseline_out="$(mktemp)"
  tmpdirs+=("${baseline_out}")
  baseline_gh_out="$(mktemp)"
  tmpdirs+=("${baseline_gh_out}")
  assert_release_baseline_failure \
    "${baseline_label} API failure" \
    "$(run_release_baseline_step "${baseline_step}" "${baseline_fixture}" api-fail "${baseline_out}" "${baseline_gh_out}")" \
    "${baseline_out}" "${baseline_gh_out}" "502"

  baseline_fixture="$(mktemp -d)"
  tmpdirs+=("${baseline_fixture}")
  write_release_baseline_fixture "${baseline_fixture}" out-of-order
  baseline_out="$(mktemp)"
  tmpdirs+=("${baseline_out}")
  baseline_gh_out="$(mktemp)"
  tmpdirs+=("${baseline_gh_out}")
  assert_release_baseline_failure \
    "${baseline_label} malformed JSON" \
    "$(run_release_baseline_step "${baseline_step}" "${baseline_fixture}" invalid-json "${baseline_out}" "${baseline_gh_out}")" \
    "${baseline_out}" "${baseline_gh_out}" ""

  baseline_fixture="$(mktemp -d)"
  tmpdirs+=("${baseline_fixture}")
  write_release_baseline_fixture "${baseline_fixture}" non-semver
  baseline_out="$(mktemp)"
  tmpdirs+=("${baseline_out}")
  baseline_gh_out="$(mktemp)"
  tmpdirs+=("${baseline_gh_out}")
  assert_release_baseline_failure \
    "${baseline_label} non-semver published tag" \
    "$(run_release_baseline_step "${baseline_step}" "${baseline_fixture}" ok "${baseline_out}" "${baseline_gh_out}")" \
    "${baseline_out}" "${baseline_gh_out}" "not X.Y.Z semver"
done

grep -Fq 'PUBLISHED_BASELINE: ${{ steps.published.outputs.baseline }}' \
  "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: release hygiene must feed the resolved released baseline to the promotion driver"
  exit 1
}

echo "release-hygiene-policy-test: PASS"
