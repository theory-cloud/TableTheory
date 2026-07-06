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

  python3 - "${repo_root}/.github/workflows/release-hygiene.yml" "${step_name}" <<'PY'
import re
import sys

workflow_path, step_name = sys.argv[1], sys.argv[2]
lines = open(workflow_path, "r", encoding="utf-8").read().splitlines()

for idx, line in enumerate(lines):
    if not re.match(r"\s*-\s+name:\s*" + re.escape(step_name) + r"\s*$", line):
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

write_v1_verifier_fixture() {
  local root="$1"

  mkdir -p "${root}/scripts"
  cat >"${root}/scripts/verify-release-cycle-state.sh" <<'SH'
#!/usr/bin/env bash
required_files=(
  "scripts/prepare-stable-promotion"".sh"
  ".release-please-manifest.premain.json"
  "py/src/theorydb_py/version.json"
)
SH
  cat >"${root}/scripts/verify-branch-release-supply-chain.sh" <<'SH'
#!/usr/bin/env bash
require_fixed '.release-please-manifest.premain.json' "${p}" \
  "prerelease workflow must reference .release-please-manifest.premain.json"
require_regex '"path"\s*:\s*"py/src/theorydb_py/version\.json"' "${cfg}" \
  "${cfg}: must bump py/src/theorydb_py/version.json version"
SH
}

write_v2_verifier_fixture() {
  local root="$1"

  mkdir -p "${root}/scripts"
  cat >"${root}/scripts/verify-release-cycle-state.sh" <<'SH'
#!/usr/bin/env bash
required_files=(
  "scripts/prepare-release-package-versions.py"
  "py/src/tabletheory_py/version.json"
)
SH
  cat >"${root}/scripts/verify-branch-release-supply-chain.sh" <<'SH'
#!/usr/bin/env bash
required_files=(
  "scripts/prepare-release-package-versions.py"
)
require_fixed "manifest-file:\s*\.release-please-manifest\.json" "${p}" \
  "prerelease workflow must reference the single release-please manifest"
require_fixed "-rc(\\.[0-9]+)?" "${provenance}" \
  "release-lane provenance guard must accept release-please first RC and numbered later RC PR titles"
require_fixed "-rc(?:\\.\\d+)?" "${prerelease_postcondition}" \
  "prerelease PR postcondition must accept release-please first RC and numbered later RC version syntax"
SH
}

write_v2_numbered_rc_verifier_fixture() {
  local root="$1"

  write_v2_verifier_fixture "${root}"
  cat >>"${root}/scripts/verify-branch-release-supply-chain.sh" <<'SH'
require_fixed "-rc\.[0-9]+" "${provenance}" \
  "release-lane provenance guard must require numbered RC release-please PR titles"
require_fixed "-rc\\.\\d+" "${prerelease_postcondition}" \
  "prerelease PR postcondition must require numbered RC version syntax"
SH
  python3 - "${root}/scripts/verify-branch-release-supply-chain.sh" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")
text = text.replace(
    'require_fixed "-rc(\\\\.[0-9]+)?" "${provenance}" \\\n'
    '  "release-lane provenance guard must accept release-please first RC and numbered later RC PR titles"\n',
    "",
)
text = text.replace(
    'require_fixed "-rc(?:\\\\.\\\\d+)?" "${prerelease_postcondition}" \\\n'
    '  "prerelease PR postcondition must accept release-please first RC and numbered later RC version syntax"\n',
    "",
)
path.write_text(text, encoding="utf-8")
PY
}

run_verifier_source_selector_fixture() {
  local trusted_shape="$1"
  local head_shape="$2"
  local base_ref="$3"
  local head_ref="$4"
  local base_repo="$5"
  local head_repo="$6"

  local fixture
  fixture="$(mktemp -d)"
  tmpdirs+=("${fixture}")

  mkdir -p "${fixture}/trusted-release" "${fixture}/pr"
  case "${trusted_shape}" in
    v1) write_v1_verifier_fixture "${fixture}/trusted-release" ;;
    v2-numbered-rc) write_v2_numbered_rc_verifier_fixture "${fixture}/trusted-release" ;;
    v2) write_v2_verifier_fixture "${fixture}/trusted-release" ;;
    *) echo "release-hygiene-policy-test: unknown trusted fixture ${trusted_shape}" >&2; exit 1 ;;
  esac
  case "${head_shape}" in
    v1) write_v1_verifier_fixture "${fixture}/pr" ;;
    v2-numbered-rc) write_v2_numbered_rc_verifier_fixture "${fixture}/pr" ;;
    v2) write_v2_verifier_fixture "${fixture}/pr" ;;
    *) echo "release-hygiene-policy-test: unknown head fixture ${head_shape}" >&2; exit 1 ;;
  esac

  local selector_script="${fixture}/selector.sh"
  extract_workflow_step_run "Resolve release verifier source" >"${selector_script}"

  local output_file="${fixture}/selector.out"
  local github_output="${fixture}/github-output"
  set +e
  (
    cd "${fixture}/pr"
    env \
      BASE_REF="${base_ref}" \
      HEAD_REF="${head_ref}" \
      BASE_REPOSITORY="${base_repo}" \
      HEAD_REPOSITORY="${head_repo}" \
      EXPECTED_REPOSITORY="${repo}" \
      GITHUB_OUTPUT="${github_output}" \
      bash "${selector_script}"
  ) >"${output_file}" 2>&1
  local status=$?
  set -e

  printf '%s\n' "${output_file}:${github_output}:${status}"
}

assert_selector_result() {
  local output_pair="$1"
  local expected_root="$2"
  local expected_label="$3"
  local expected_output="$4"

  local output_file="${output_pair%%:*}"
  local remainder="${output_pair#*:}"
  local github_output="${remainder%%:*}"
  local status="${remainder#*:}"

  if [[ "${status}" -ne 0 ]]; then
    cat "${output_file}"
    echo "release-hygiene-policy-test: selector expected success, got ${status}"
    exit 1
  fi

  if ! grep -Fxq "root=${expected_root}" "${github_output}"; then
    cat "${output_file}"
    cat "${github_output}"
    echo "release-hygiene-policy-test: selector root mismatch; expected ${expected_root}"
    exit 1
  fi
  if ! grep -Fxq "label=${expected_label}" "${github_output}"; then
    cat "${output_file}"
    cat "${github_output}"
    echo "release-hygiene-policy-test: selector label mismatch; expected ${expected_label}"
    exit 1
  fi
  if ! grep -Fq "${expected_output}" "${output_file}"; then
    cat "${output_file}"
    echo "release-hygiene-policy-test: selector output missing: ${expected_output}"
    exit 1
  fi
}

assert_selector_failure() {
  local output_pair="$1"
  local expected_output="$2"

  local output_file="${output_pair%%:*}"
  local remainder="${output_pair#*:}"
  local github_output="${remainder%%:*}"
  local status="${remainder#*:}"

  if [[ "${status}" -eq 0 ]]; then
    cat "${output_file}"
    cat "${github_output}"
    echo "release-hygiene-policy-test: selector expected failure"
    exit 1
  fi
  if ! grep -Fq "${expected_output}" "${output_file}"; then
    cat "${output_file}"
    echo "release-hygiene-policy-test: selector failure output missing: ${expected_output}"
    exit 1
  fi
}

current_policy_branch_name() {
  if [[ -n "${GITHUB_HEAD_REF:-}" ]]; then
    printf '%s\n' "${GITHUB_HEAD_REF}"
    return 0
  fi

  git rev-parse --abbrev-ref HEAD 2>/dev/null || true
}

validate_policy_bootstrap_scope() {
  local base_ref="${BOOTSTRAP_BASE_REF:-origin/main}"
  local changed_files=()
  local untracked_files=()
  local changed_file

  if ! git rev-parse --verify --quiet "${base_ref}^{commit}" >/dev/null; then
    if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
      echo "release-hygiene-policy-test: scoped bootstrap diff scope is enforced by the workflow guard"
      return 0
    fi
    echo "release-hygiene-policy-test: missing ${base_ref} for scoped bootstrap diff validation"
    exit 1
  fi

  mapfile -t changed_files < <(
    {
      git diff --name-only "${base_ref}...HEAD"
      git diff --name-only
      git diff --cached --name-only
    } | sort -u
  )
  mapfile -t untracked_files < <(git ls-files --others --exclude-standard)

  if [[ "${#untracked_files[@]}" -ne 0 ]]; then
    printf 'release-hygiene-policy-test: untracked file blocks scoped bootstrap: %s\n' "${untracked_files[@]}"
    exit 1
  fi

  if [[ "${#changed_files[@]}" -eq 0 ]]; then
    echo "release-hygiene-policy-test: scoped bootstrap branch has no changed files"
    exit 1
  fi

  for changed_file in "${changed_files[@]}"; do
    case "${changed_file}" in
      .github/workflows/release-hygiene.yml|\
      scripts/verify-release-lane-provenance.sh|\
      scripts/verify-promotion-release-driver.sh|\
      scripts/test-release-hygiene-policy.sh|\
      scripts/verify-release-cycle-state.sh|\
      scripts/verify-branch-release-supply-chain.sh)
        ;;
      *)
        echo "release-hygiene-policy-test: unexpected scoped bootstrap file ${changed_file}"
        exit 1
        ;;
    esac
  done
}

run_main_bootstrap_policy_tests() {
  validate_policy_bootstrap_scope

  grep -Fq "Resolve release verifier source" "${repo_root}/.github/workflows/release-hygiene.yml" || {
    echo "release-hygiene-policy-test: release hygiene must resolve verifier source"
    exit 1
  }
  grep -Fq "protected-pr-head-v2" "${repo_root}/.github/workflows/release-hygiene.yml" || {
    echo "release-hygiene-policy-test: release hygiene must preserve protected PR-head v2 fallback"
    exit 1
  }
  grep -Fq "accept release-please first RC and numbered later RC PR titles" "${repo_root}/.github/workflows/release-hygiene.yml" || {
    echo "release-hygiene-policy-test: release hygiene must feature-detect RC-first PR-title support"
    exit 1
  }
  grep -Fq "accept release-please first RC and numbered later RC version syntax" "${repo_root}/.github/workflows/release-hygiene.yml" || {
    echo "release-hygiene-policy-test: release hygiene must feature-detect RC-first version support"
    exit 1
  }
  grep -Fq 'bash "${VERIFIER_ROOT}/scripts/verify-release-cycle-state.sh"' "${repo_root}/.github/workflows/release-hygiene.yml" || {
    echo "release-hygiene-policy-test: release-cycle state must use the resolved verifier source"
    exit 1
  }
  grep -Fq 'bash "${VERIFIER_ROOT}/scripts/verify-branch-release-supply-chain.sh"' "${repo_root}/.github/workflows/release-hygiene.yml" || {
    echo "release-hygiene-policy-test: branch release supply-chain must use the resolved verifier source"
    exit 1
  }

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

  expect_success_contains \
    "RC Release-As 1.9.3-rc" \
    bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
      --base premain \
      --head staging \
      --title "Promote staging to premain" \
      --body "Release-As: 1.9.3-rc" \
      --dry-run

  pending_fixture="$(mktemp -d)"
  tmpdirs+=("${pending_fixture}")
  mkdir -p \
    "${pending_fixture}/.github/workflows" \
    "${pending_fixture}/scripts" \
    "${pending_fixture}/ts" \
    "${pending_fixture}/py/src/tabletheory_py"
  cat >"${pending_fixture}/.release-please-manifest.json" <<'JSON'
{".":"1.10.1-rc"}
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

  expect_success_contains \
    "rc=1.10.1-rc" \
    env \
      GITHUB_BASE_REF=main \
      GITHUB_HEAD_REF=premain \
      RELEASE_CYCLE_ALLOW_PENDING_STABLE_PROMOTION=true \
      RELEASE_CYCLE_REPO_ROOT="${pending_fixture}" \
      bash "${repo_root}/scripts/verify-release-cycle-state.sh"

  (
    cd "${pending_fixture}"
    expect_success_contains \
      "premain -> main pending stable promotion 1.10.1-rc -> 1.10.1" \
      bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
        --base main \
        --head premain \
        --title "Promote premain to main" \
        --body "Release-As: 1.10.1" \
        --dry-run
  )

  selector_result="$(
    run_verifier_source_selector_fixture \
      v1 v2 \
      main premain \
      "${repo}" "${repo}"
  )"
  assert_selector_result \
    "${selector_result}" \
    "." \
    "protected-pr-head-v2" \
    "protected same-repo promotion may use PR-head v2/RC-first verifier scripts after provenance"

  bash "${repo_root}/scripts/verify-branch-release-supply-chain.sh" >/dev/null

  echo "release-hygiene-policy-test: PASS (scoped main release-hygiene bootstrap)"
  exit 0
}

if [[ "$(current_policy_branch_name)" == fix/release-hygiene-main-bootstrap-* ]]; then
  run_main_bootstrap_policy_tests
fi

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
  "live ref freshness covered by merge queue" \
  bash "${checker}" \
    --repo "${repo}" \
    --base premain \
    --head staging \
    --base-repo "${repo}" \
    --head-repo "${repo}" \
    --base-sha "${base_sha}" \
    --head-sha "${head_sha}" \
    --title "Promote staging to premain" \
    --queue-freshness \
    --ref "refs/heads/premain=${advanced_base_sha}" \
    --ref "refs/heads/staging=4444444444444444444444444444444444444444"

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
    --queue-freshness

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

expect_failure_contains \
  "requires a stable Release-As footer" \
  run_in_pending_fixture \
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

if grep -Fq "secrets.RELEASE_PLEASE_TOKEN" "${repo_root}/.github/workflows/release-hygiene.yml"; then
  echo "release-hygiene-policy-test: release hygiene must not expose release-please token"
  exit 1
fi

grep -Fq "persist-credentials: false" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: release hygiene checkouts must disable persisted credentials"
  exit 1
}

grep -Fq "merge_group:" "${repo_root}/.github/workflows/quality-gates.yml" || {
  echo "release-hygiene-policy-test: quality gates must support merge_group for staging queue"
  exit 1
}

grep -Fq "merge_group:" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: release hygiene must support merge_group for release queue"
  exit 1
}

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

grep -Fq -- "--queue-freshness" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: release hygiene PR provenance must delegate live ref freshness to merge queue"
  exit 1
}

grep -Fq "verify-release-lane-provenance.sh --help" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: release hygiene must feature-detect trusted provenance flags"
  exit 1
}

grep -Fq "provenance_args+=(--queue-freshness)" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: release hygiene must append --queue-freshness only after feature detection"
  exit 1
}

grep -Fq "trusted base script lacks --queue-freshness" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: release hygiene must document strict-freshness fallback"
  exit 1
}

grep -Fq "Resolve release verifier source" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: release hygiene must resolve verifier source for PR checks"
  exit 1
}

grep -Fq "premain:staging|main:premain" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: verifier fallback must be limited to protected promotion branch pairs"
  exit 1
}

grep -Fq "trusted base lacks v2 single-manifest support or RC-first verifier support" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: verifier selector must document old trusted-script and RC-first fallback reasons"
  exit 1
}

grep -Fq "protected-pr-head-v2" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: verifier selector must label protected PR-head v2 fallback"
  exit 1
}

grep -Fq "scripts/prepare-release-package-versions.py" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: verifier selector must feature-detect the v2 package-version marker"
  exit 1
}

grep -Fq "single release-please manifest" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: verifier selector must feature-detect the v2 single-manifest marker"
  exit 1
}

grep -Fq "accept release-please first RC and numbered later RC PR titles" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: verifier selector must feature-detect the RC-first PR-title marker"
  exit 1
}

grep -Fq "accept release-please first RC and numbered later RC version syntax" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: verifier selector must feature-detect the RC-first version marker"
  exit 1
}

grep -Fq 'bash "${VERIFIER_ROOT}/scripts/verify-release-cycle-state.sh"' "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: release-cycle state must use the resolved verifier source"
  exit 1
}

grep -Fq 'bash "${VERIFIER_ROOT}/scripts/verify-branch-release-supply-chain.sh"' "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: branch release supply-chain must use the resolved verifier source"
  exit 1
}

selector_result="$(
  run_verifier_source_selector_fixture \
    v1 v2 \
    premain staging \
    "${repo}" "${repo}"
)"
assert_selector_result \
  "${selector_result}" \
  "." \
  "protected-pr-head-v2" \
  "protected same-repo promotion may use PR-head v2/RC-first verifier scripts after provenance"

selector_result="$(
  run_verifier_source_selector_fixture \
    v2-numbered-rc v2 \
    premain staging \
    "${repo}" "${repo}"
)"
assert_selector_result \
  "${selector_result}" \
  "." \
  "protected-pr-head-v2" \
  "protected same-repo promotion may use PR-head v2/RC-first verifier scripts after provenance"

selector_result="$(
  run_verifier_source_selector_fixture \
    v1 v2 \
    premain "feature/arbitrary-head" \
    "${repo}" "${repo}"
)"
assert_selector_result \
  "${selector_result}" \
  "../trusted-release" \
  "trusted-base" \
  "using trusted base verifier scripts"

selector_result="$(
  run_verifier_source_selector_fixture \
    v2-numbered-rc v2 \
    premain "feature/arbitrary-head" \
    "${repo}" "${repo}"
)"
assert_selector_result \
  "${selector_result}" \
  "../trusted-release" \
  "trusted-base" \
  "using trusted base verifier scripts"

selector_result="$(
  run_verifier_source_selector_fixture \
    v1 v2 \
    premain staging \
    "${repo}" "attacker/TableTheory"
)"
assert_selector_result \
  "${selector_result}" \
  "../trusted-release" \
  "trusted-base" \
  "using trusted base verifier scripts"

selector_result="$(
  run_verifier_source_selector_fixture \
    v2-numbered-rc v2 \
    premain staging \
    "${repo}" "attacker/TableTheory"
)"
assert_selector_result \
  "${selector_result}" \
  "../trusted-release" \
  "trusted-base" \
  "using trusted base verifier scripts"

selector_result="$(
  run_verifier_source_selector_fixture \
    v2 v2 \
    premain staging \
    "${repo}" "${repo}"
)"
assert_selector_result \
  "${selector_result}" \
  "../trusted-release" \
  "trusted-base" \
  "trusted base supports v2 single-manifest and RC-first verifier markers"

selector_result="$(
  run_verifier_source_selector_fixture \
    v2-numbered-rc v2-numbered-rc \
    premain staging \
    "${repo}" "${repo}"
)"
assert_selector_failure \
  "${selector_result}" \
  "protected PR head lacks v2 single-manifest support or RC-first verifier support"

grep -Fq "pending stable promotion accepted on queued main merge group" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: release hygiene must allow queued main pending stable promotion"
  exit 1
}

grep -Fq "../trusted-release/scripts/verify-promotion-release-driver.sh" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: promotion driver must run from trusted checkout"
  exit 1
}

grep -Fq "Verify release-hygiene bootstrap scope" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: main bootstrap PRs must have a scoped hygiene guard"
  exit 1
}

grep -Fq "fix/release-hygiene-main-bootstrap-" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: main bootstrap guard must be branch-scoped"
  exit 1
}

grep -Fq "pulls/\${PR_NUMBER}/files" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: main bootstrap guard must inspect PR changed files"
  exit 1
}

bootstrap_scope_section="$(sed -n '/Verify release-hygiene bootstrap scope/,/Verify release-lane same-repository provenance/p' "${repo_root}/.github/workflows/release-hygiene.yml")"
grep -Fq "scripts/verify-release-cycle-state.sh" <<<"${bootstrap_scope_section}" || {
  echo "release-hygiene-policy-test: main bootstrap guard must allow verify-release-cycle-state bootstrap repairs"
  exit 1
}

grep -Fq 'RELEASE_CYCLE_REPO_ROOT: ${{ github.workspace }}/pr' "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: release cycle state must validate the checked-out PR head"
  exit 1
}

echo "release-hygiene-policy-test: PASS"
