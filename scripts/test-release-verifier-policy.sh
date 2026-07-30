#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fixtures_dir="${repo_root}/scripts/fixtures/release-policy"
tmpdirs=()

cleanup() {
  local tmpdir
  for tmpdir in "${tmpdirs[@]}"; do
    rm -rf "${tmpdir}"
  done
}
trap cleanup EXIT

expect_contains() {
  local output="$1"
  local expected="$2"
  local context="$3"
  if [[ -n "${expected}" ]] && ! grep -Fq "${expected}" <<<"${output}"; then
    printf '%s\n' "${output}"
    echo "release-verifier-policy-test: ${context}: expected output to contain: ${expected}"
    exit 1
  fi
}

expect_absent() {
  local output="$1"
  local unexpected="$2"
  local context="$3"
  if [[ -n "${unexpected}" ]] && grep -Fq "${unexpected}" <<<"${output}"; then
    printf '%s\n' "${output}"
    echo "release-verifier-policy-test: ${context}: unexpected output: ${unexpected}"
    exit 1
  fi
}

write_version_files() {
  local root="$1"
  local stable="$2"
  local _retired_premain="$3"
  local ts="$4"
  local py="$5"
  local toolchain="${6:-go1.26.5}"
  local py_path_mode="${7:-canonical}"
  local go_major module_path

  mkdir -p \
    "${root}/ts" \
    "${root}/scripts" \
    "${root}/.github/workflows" \
    "${root}/examples/multi-tenant"

  printf '{".":"%s"}\n' "${stable}" >"${root}/.release-please-manifest.json"
  printf '{"version":"%s"}\n' "${ts}" >"${root}/ts/package.json"
  printf '{"version":"%s","packages":{"":{"version":"%s"}}}\n' "${ts}" "${ts}" >"${root}/ts/package-lock.json"
  case "${py_path_mode}" in
    canonical)
      mkdir -p "${root}/py/src/tabletheory_py"
      printf '{"version":"%s"}\n' "${py}" >"${root}/py/src/tabletheory_py/version.json"
      ;;
    legacy)
      mkdir -p "${root}/py/src/theorydb_py"
      printf '{"version":"%s"}\n' "${py}" >"${root}/py/src/theorydb_py/version.json"
      ;;
    missing)
      ;;
    malformed-canonical)
      mkdir -p "${root}/py/src/tabletheory_py"
      printf '{"version":' >"${root}/py/src/tabletheory_py/version.json"
      ;;
    malformed-legacy)
      mkdir -p "${root}/py/src/theorydb_py"
      printf '{"version":' >"${root}/py/src/theorydb_py/version.json"
      ;;
    *)
      echo "release-verifier-policy-test: unknown py_path_mode ${py_path_mode}"
      exit 1
      ;;
  esac
  go_major="${stable#v}"
  go_major="${go_major%%.*}"
  module_path="github.com/theory-cloud/tabletheory"
  if [[ "${go_major}" =~ ^[0-9]+$ && "${go_major}" -ge 2 ]]; then
    module_path="${module_path}/v${go_major}"
  fi
  printf 'module %s\n\ngo 1.26\ntoolchain %s\n' "${module_path}" "${toolchain}" >"${root}/go.mod"
  printf 'module fixture/example\n\ngo 1.26\ntoolchain %s\n' "${toolchain}" >"${root}/examples/multi-tenant/go.mod"

  cat >"${root}/scripts/prepare-release-package-versions.py" <<'STUB'
#!/usr/bin/env python3
print("release-package-versions: PASS (fixture)")
STUB
  cat >"${root}/scripts/verify-release-package-version-assets.py" <<'STUB'
#!/usr/bin/env python3
print("release-package-version-assets: PASS (fixture)")
STUB
  cat >"${root}/scripts/verify-go-semantic-import-version.sh" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
echo "go-semantic-import: PASS (fixture stub)"
STUB
  cat >"${root}/scripts/watch-release-cycle.sh" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
echo "watch-release-cycle: PASS (fixture stub)"
STUB
  : >"${root}/.github/workflows/release.yml"
}

load_fixture() {
  local fixture="$1"
  # shellcheck disable=SC1090
  source "${fixture}/fixture.env"
}

run_cycle_fixture() {
  local fixture="$1"
  unset kind branch head_ref pending_env stable premain ts py expected_exit expected_text premain_rc_repair staging_rc_followup main_stable
  load_fixture "${fixture}"

  local work output status
  work="$(mktemp -d)"
  tmpdirs+=("${work}")
  if [[ "${staging_rc_followup:-false}" == "true" ]]; then
    git -C "${work}" init -q
    git -C "${work}" config user.email fixture@example.com
    git -C "${work}" config user.name "Release Fixture"

    write_version_files "${work}" "${main_stable}" "" "${ts}" "${py}"
    git -C "${work}" add .
    git -C "${work}" commit -q -m "fixture main"
    git -C "${work}" update-ref refs/remotes/origin/main HEAD
    git -C "${work}" branch -M staging

    write_version_files "${work}" "${stable}" "${premain}" "${ts}" "${py}"
    git -C "${work}" add .
    git -C "${work}" commit -q -m "fixture accepted staging rc"
    git -C "${work}" update-ref refs/remotes/origin/staging HEAD

    git -C "${work}" checkout -q -b fixture/from-staging
    git -C "${work}" commit -q --allow-empty -m "fixture staging rc follow-up repair"
  elif [[ "${premain_rc_repair:-false}" == "true" ]]; then
    git -C "${work}" init -q
    git -C "${work}" config user.email fixture@example.com
    git -C "${work}" config user.name "Release Fixture"

    write_version_files "${work}" "${main_stable}" "" "${ts}" "${py}"
    git -C "${work}" add .
    git -C "${work}" commit -q -m "fixture main"
    git -C "${work}" update-ref refs/remotes/origin/main HEAD
    git -C "${work}" branch -M staging
    git -C "${work}" update-ref refs/remotes/origin/staging HEAD

    write_version_files "${work}" "${stable}" "${premain}" "${ts}" "${py}"
    git -C "${work}" add .
    git -C "${work}" commit -q -m "fixture premain rc"
    git -C "${work}" update-ref refs/remotes/origin/premain HEAD

    git -C "${work}" checkout -q -b fixture/from-premain
    git -C "${work}" commit -q --allow-empty -m "fixture premain rc repair"
  else
    write_version_files "${work}" "${stable}" "${premain}" "${ts}" "${py}"
  fi

  set +e
  output="$(
    GITHUB_REF_NAME="${branch}" \
    GITHUB_HEAD_REF="${head_ref}" \
    RELEASE_CYCLE_ALLOW_PENDING_STABLE_PROMOTION="${pending_env}" \
      bash "${repo_root}/scripts/verify-release-cycle-state.sh" --repo-root "${work}" 2>&1
  )"
  status=$?
  set -e

  if [[ "${status}" -ne "${expected_exit}" ]]; then
    printf '%s\n' "${output}"
    echo "release-verifier-policy-test: $(basename "${fixture}"): expected exit ${expected_exit}, got ${status}"
    exit 1
  fi
  expect_contains "${output}" "${expected_text}" "$(basename "${fixture}")"
}

commit_watch_branch() {
  local repo="$1"
  local branch="$2"
  local stable="$3"
  local premain="$4"
  local ts="$5"
  local py="$6"
  local toolchain="$7"
  local py_path_mode="${8:-canonical}"

  git -C "${repo}" rm -r --quiet . >/dev/null 2>&1 || true
  write_version_files "${repo}" "${stable}" "${premain}" "${ts}" "${py}" "${toolchain}" "${py_path_mode}"
  git -C "${repo}" add .
  git -C "${repo}" commit -q --allow-empty -m "fixture ${branch}"
  git -C "${repo}" update-ref "refs/remotes/origin/${branch}" HEAD
}

run_watch_fixture() {
  local fixture="$1"
  unset kind strict main_stable main_premain main_ts main_py main_py_path_mode premain_stable premain_prerelease premain_py_path_mode staging_stable staging_py_path_mode toolchain expected_exit expected_pass expected_warn expected_fail
  load_fixture "${fixture}"

  local work output status strict_args=()
  work="$(mktemp -d)"
  tmpdirs+=("${work}")
  git -C "${work}" init -q
  git -C "${work}" config user.email fixture@example.com
  git -C "${work}" config user.name "Release Fixture"

  commit_watch_branch "${work}" main "${main_stable}" "${main_premain}" "${main_ts}" "${main_py}" "${toolchain}" "${main_py_path_mode:-canonical}"
  commit_watch_branch "${work}" premain "${premain_prerelease}" "" "${main_ts}" "${main_py}" "${toolchain}" "${premain_py_path_mode:-canonical}"
  commit_watch_branch "${work}" staging "${staging_stable}" "${staging_stable}" "${staging_stable}" "${staging_stable}" "${toolchain}" "${staging_py_path_mode:-canonical}"

  if [[ "${strict}" == "true" ]]; then
    strict_args=(--strict)
  fi

  set +e
  output="$(bash "${repo_root}/scripts/watch-release-cycle.sh" --repo-root "${work}" --skip-github "${strict_args[@]}" 2>&1)"
  status=$?
  set -e

  if [[ "${status}" -ne "${expected_exit}" ]]; then
    printf '%s\n' "${output}"
    echo "release-verifier-policy-test: $(basename "${fixture}"): expected exit ${expected_exit}, got ${status}"
    exit 1
  fi
  expect_contains "${output}" "${expected_pass}" "$(basename "${fixture}")"
  expect_contains "${output}" "${expected_warn}" "$(basename "${fixture}")"
  expect_contains "${output}" "${expected_fail}" "$(basename "${fixture}")"
  expect_absent "${output}" "Traceback" "$(basename "${fixture}")"
  expect_absent "${output}" "JSONDecodeError" "$(basename "${fixture}")"
  if [[ -z "${expected_fail}" ]]; then
    expect_absent "${output}" "watch-release-cycle: FAIL" "$(basename "${fixture}")"
  fi
}

run_branch_version_sync_fixture() {
  local candidate_version="$1"
  local expected_status="$2"
  local expected_output="$3"

  local work remote output status
  work="$(mktemp -d)"
  remote="$(mktemp -d)"
  tmpdirs+=("${work}" "${remote}")

  git -C "${remote}" init --bare -q
  git -C "${work}" init -q
  git -C "${work}" config user.email fixture@example.com
  git -C "${work}" config user.name "Release Fixture"

  write_version_files "${work}" "1.10.1" "" "1.10.0" "1.10.0"
  git -C "${work}" add .
  git -C "${work}" commit -q -m "fixture main"
  git -C "${work}" branch -M main
  git -C "${work}" remote add origin "${remote}"
  git -C "${remote}" fetch -q "${work}" HEAD:refs/heads/main

  write_version_files "${work}" "${candidate_version}" "" "1.10.0" "1.10.0"
  git -C "${work}" add .
  git -C "${work}" commit -q -m "fixture premain"

  set +e
  output="$(
    GIT_FETCH_RETRIES=1 \
    GITHUB_REF_NAME=premain \
      bash "${repo_root}/scripts/verify-branch-version-sync.sh" --repo-root "${work}" 2>&1
  )"
  status=$?
  set -e

  if [[ "${status}" -ne "${expected_status}" ]]; then
    printf '%s\n' "${output}"
    echo "release-verifier-policy-test: branch-version-sync fixture ${candidate_version}: expected ${expected_status}, got ${status}"
    exit 1
  fi
  expect_contains \
    "${output}" \
    "${expected_output}" \
    "branch-version-sync fixture ${candidate_version}"
  expect_absent "${output}" "JSONDecodeError" "branch-version-sync fixture"
}

run_branch_version_sync_fixture \
  "1.10.1-rc" \
  0 \
  "branch-version-sync: PASS (main=1.10.1, candidate=1.10.1-rc, mode=premain)"

run_branch_version_sync_fixture \
  "1.10.1-rc.1" \
  0 \
  "branch-version-sync: PASS (main=1.10.1, candidate=1.10.1-rc.1, mode=premain)"

run_branch_version_sync_fixture \
  "1.10.1-beta.1" \
  1 \
  "branch-version-sync: FAIL (checked-out .release-please-manifest.json has invalid version '1.10.1-beta.1')"

run_branch_version_sync_fixture \
  "1.10.1-rc." \
  1 \
  "branch-version-sync: FAIL (checked-out .release-please-manifest.json has invalid version '1.10.1-rc.')"

run_go_semantic_import_fixture() {
  local manifest="$1"
  local module_path="$2"
  local expected_status="$3"
  local expected_output="$4"

  local work output status
  work="$(mktemp -d)"
  tmpdirs+=("${work}")
  mkdir -p "${work}"
  printf '{".":"%s"}\n' "${manifest}" >"${work}/.release-please-manifest.json"
  printf 'module %s\n\ngo 1.26\n' "${module_path}" >"${work}/go.mod"

  set +e
  output="$(bash "${repo_root}/scripts/verify-go-semantic-import-version.sh" --repo-root "${work}" 2>&1)"
  status=$?
  set -e

  if [[ "${status}" -ne "${expected_status}" ]]; then
    printf '%s\n' "${output}"
    echo "release-verifier-policy-test: go-semantic-import fixture ${manifest}/${module_path}: expected ${expected_status}, got ${status}"
    exit 1
  fi
  expect_contains "${output}" "${expected_output}" "go-semantic-import fixture"
}

run_go_pending_semantic_import_fixture() {
  local branch="$1"
  local with_evidence="$2"
  local expected_status="$3"
  local expected_output="$4"

  local work output status
  work="$(mktemp -d)"
  tmpdirs+=("${work}")
  printf '{".":"2.0.6"}\n' >"${work}/.release-please-manifest.json"
  printf 'module github.com/theory-cloud/tabletheory/v3\n\ngo 1.26\n' >"${work}/go.mod"
  if [[ "${with_evidence}" == "true" ]]; then
    mkdir -p "${work}/docs/migration"
    cat >"${work}/CHANGELOG.md" <<'EOF'
# Changelog

## [Unreleased]

### Breaking Changes

* **contract:** advance to DMS v0.2.
EOF
    printf '# TableTheory v3 migration\n' >"${work}/docs/migration/v3.md"
  fi

  set +e
  output="$(
    GITHUB_BASE_REF="${branch}" \
      bash "${repo_root}/scripts/verify-go-semantic-import-version.sh" --repo-root "${work}" 2>&1
  )"
  status=$?
  set -e

  if [[ "${status}" -ne "${expected_status}" ]]; then
    printf '%s\n' "${output}"
    echo "release-verifier-policy-test: pending semantic import fixture ${branch}/${with_evidence}: expected ${expected_status}, got ${status}"
    exit 1
  fi
  expect_contains "${output}" "${expected_output}" "pending semantic import fixture"
}

run_go_semantic_import_fixture \
  "2.0.1" \
  "github.com/theory-cloud/tabletheory/v2" \
  0 \
  "go-semantic-import: PASS"

run_go_semantic_import_fixture \
  "3.0.0-rc.1" \
  "github.com/theory-cloud/tabletheory/v3" \
  0 \
  "go-semantic-import: PASS"

run_go_semantic_import_fixture \
  "2.0.1" \
  "github.com/theory-cloud/tabletheory" \
  1 \
  "manifest 2.0.1 requires module github.com/theory-cloud/tabletheory/v2"

run_go_semantic_import_fixture \
  "1.10.1" \
  "github.com/theory-cloud/tabletheory/v2" \
  1 \
  "manifest 1.10.1 requires module github.com/theory-cloud/tabletheory"

run_go_semantic_import_fixture \
  "2.0.6" \
  "github.com/theory-cloud/tabletheory/v4" \
  1 \
  "manifest 2.0.6 requires module github.com/theory-cloud/tabletheory/v2"

run_go_pending_semantic_import_fixture \
  "staging" \
  "true" \
  0 \
  "pending-major-transition=2->3"

run_go_pending_semantic_import_fixture \
  "premain" \
  "true" \
  0 \
  "pending-major-transition=2->3"

run_go_pending_semantic_import_fixture \
  "main" \
  "true" \
  1 \
  "pending v3 semantic import transition is not allowed on main"

run_go_pending_semantic_import_fixture \
  "develop" \
  "true" \
  1 \
  "allowed only on PRs targeting staging or premain"

run_go_pending_semantic_import_fixture \
  "staging" \
  "false" \
  1 \
  "pending v3 transition requires CHANGELOG.md"

for fixture in "${fixtures_dir}"/cycle-*; do
  run_cycle_fixture "${fixture}"
done

for fixture in "${fixtures_dir}"/watch-*; do
  run_watch_fixture "${fixture}"
done

echo "release-verifier-policy-test: PASS"
