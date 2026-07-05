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
  local toolchain="${6:-go1.26.4}"

  mkdir -p \
    "${root}/ts" \
    "${root}/py/src/theorydb_py" \
    "${root}/scripts" \
    "${root}/.github/workflows" \
    "${root}/examples/multi-tenant"

  printf '{".":"%s"}\n' "${stable}" >"${root}/.release-please-manifest.json"
  printf '{"version":"%s"}\n' "${ts}" >"${root}/ts/package.json"
  printf '{"version":"%s","packages":{"":{"version":"%s"}}}\n' "${ts}" "${ts}" >"${root}/ts/package-lock.json"
  printf '{"version":"%s"}\n' "${py}" >"${root}/py/src/theorydb_py/version.json"
  printf 'module fixture\n\ngo 1.26\ntoolchain %s\n' "${toolchain}" >"${root}/go.mod"
  printf 'module fixture/example\n\ngo 1.26\ntoolchain %s\n' "${toolchain}" >"${root}/examples/multi-tenant/go.mod"

  cat >"${root}/scripts/prepare-stable-promotion.sh" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
echo "prepare-stable-promotion: PASS (fixture)"
STUB
  cat >"${root}/scripts/prepare-release-package-versions.py" <<'STUB'
#!/usr/bin/env python3
print("release-package-versions: PASS (fixture)")
STUB
  cat >"${root}/scripts/verify-release-package-version-assets.py" <<'STUB'
#!/usr/bin/env python3
print("release-package-version-assets: PASS (fixture)")
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
  unset kind branch head_ref pending_env stable premain ts py expected_exit expected_text
  load_fixture "${fixture}"

  local work output status
  work="$(mktemp -d)"
  tmpdirs+=("${work}")
  write_version_files "${work}" "${stable}" "${premain}" "${ts}" "${py}"

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

  git -C "${repo}" rm -r --quiet . >/dev/null 2>&1 || true
  write_version_files "${repo}" "${stable}" "${premain}" "${ts}" "${py}" "${toolchain}"
  git -C "${repo}" add .
  git -C "${repo}" commit -q --allow-empty -m "fixture ${branch}"
  git -C "${repo}" update-ref "refs/remotes/origin/${branch}" HEAD
}

run_watch_fixture() {
  local fixture="$1"
  unset kind strict main_stable main_premain main_ts main_py premain_stable premain_prerelease staging_stable toolchain expected_exit expected_pass expected_warn expected_fail
  load_fixture "${fixture}"

  local work output status strict_args=()
  work="$(mktemp -d)"
  tmpdirs+=("${work}")
  git -C "${work}" init -q
  git -C "${work}" config user.email fixture@example.com
  git -C "${work}" config user.name "Release Fixture"

  commit_watch_branch "${work}" main "${main_stable}" "${main_premain}" "${main_ts}" "${main_py}" "${toolchain}"
  commit_watch_branch "${work}" premain "${premain_prerelease}" "" "${main_ts}" "${main_py}" "${toolchain}"
  commit_watch_branch "${work}" staging "${staging_stable}" "${staging_stable}" "${staging_stable}" "${staging_stable}" "${toolchain}"

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
  if [[ -z "${expected_fail}" ]]; then
    expect_absent "${output}" "watch-release-cycle: FAIL" "$(basename "${fixture}")"
  fi
}

for fixture in "${fixtures_dir}"/cycle-*; do
  run_cycle_fixture "${fixture}"
done

for fixture in "${fixtures_dir}"/watch-*; do
  run_watch_fixture "${fixture}"
done

echo "release-verifier-policy-test: PASS"
