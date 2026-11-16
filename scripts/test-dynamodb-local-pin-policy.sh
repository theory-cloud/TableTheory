#!/usr/bin/env bash
set -euo pipefail

# dynamodb-local-pin-guard: file-owns-image-fixtures
#
# The image strings below are fixtures this test feeds to the COM-7 guard, not
# installs it performs. The guard skips a file that declares the marker above, so
# this test can embed unpinned references. A regression here can still never turn
# the guard vacuous: the red cases assert that an unpinned compose file, Makefile
# or shell script is rejected, and the green cases assert that the same shapes
# pass once they carry a digest.
#
# Policy test for scripts/verify-dynamodb-local-pin.sh (rubric COM-7):
#
#   - every DynamoDB Local image reference carries an @sha256 digest
#   - compose files, Makefiles, shell scripts, templates and profile JSON are all
#     in scope
#   - a local runner publishes DynamoDB Local on loopback only
#
# Each case builds a throwaway tree, runs the real guard against it, and asserts
# both the exit status and the emitted message.

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
guard="${repo_root}/scripts/verify-dynamodb-local-pin.sh"

if [[ ! -f "${guard}" ]]; then
  echo "dynamodb-local-pin-policy: FAIL (missing ${guard})"
  exit 1
fi

failures=0

scratch="$(mktemp -d)"
trap 'rm -rf "${scratch}"' EXIT

case_dir=""
output=""
status=0

digest="sha256:7ef4a2c45b58c2901e70a4f28e0953a422c2c631baaaf5e2c15e0805740c7752"
pinned="amazon/dynamodb-local:3.1.0@${digest}"
unpinned="amazon/dynamodb-local:3.1.0"

begin_case() {
  case_dir="$(mktemp -d "${scratch}/case.XXXXXX")"
  printf 'module example.com/fixture\n\ngo 1.26\n' >"${case_dir}/go.mod"
}

run_guard() {
  set +e
  output="$(bash "${guard}" --root "${case_dir}" 2>&1)"
  status=$?
  set -e
}

report_failure() {
  echo "dynamodb-local-pin-policy: FAIL ($1)"
  printf '%s\n' "${output}" | sed 's/^/    /'
  failures=$((failures + 1))
}

expect_status() {
  local expected="$1" label="$2"
  if [[ "${expected}" == PASS && "${status}" -ne 0 ]]; then
    report_failure "${label}"
  elif [[ "${expected}" == FAIL && "${status}" -eq 0 ]]; then
    report_failure "${label}"
  fi
}

expect_message() {
  local needle="$1" label="$2"
  if ! grep -Fq -- "${needle}" <<< "${output}"; then
    report_failure "${label} (missing message: ${needle})"
  fi
}

# 1. A compose file that names the tag without a digest is rejected.
begin_case
cat >"${case_dir}/docker-compose.yml" <<YAML
services:
  dynamodb-local:
    image: ${unpinned}
YAML
run_guard
expect_status FAIL "compose file with a mutable tag"
expect_message "docker-compose.yml:3" "compose file with a mutable tag names the line"
expect_message "not pinned" "compose file with a mutable tag"

# 2. The Makefile default is in scope: it is what `make integration` starts.
begin_case
printf 'DYNAMODB_LOCAL_IMAGE ?= %s\n' "${unpinned}" >"${case_dir}/Makefile"
run_guard
expect_status FAIL "Makefile default with a mutable tag"
expect_message "Makefile:1" "Makefile default with a mutable tag names the line"

# 3. A shell script that starts the container is in scope.
begin_case
mkdir -p "${case_dir}/scripts"
cat >"${case_dir}/scripts/start.sh" <<SH
#!/usr/bin/env bash
docker run -d -p 8000:8000 ${unpinned} -jar DynamoDBLocal.jar
SH
run_guard
expect_status FAIL "shell script with a mutable tag"
expect_message "scripts/start.sh:2" "shell script with a mutable tag names the line"

# 4. An untagged reference is an implicit :latest, which is equally mutable.
begin_case
mkdir -p "${case_dir}/scripts"
printf '#!/usr/bin/env bash\ndocker run -d amazon/dynamodb-local -jar DynamoDBLocal.jar\n' >"${case_dir}/scripts/start.sh"
run_guard
expect_status FAIL "untagged reference"
expect_message "scripts/start.sh:2" "untagged reference names the line"

# 5. Templates and the checked-in profile JSON are in scope too.
begin_case
mkdir -p "${case_dir}/cmd/tabletheory/templates/shared" "${case_dir}/scripts/local"
printf 'services:\n  ddb:\n    image: %s\n' "${unpinned}" >"${case_dir}/cmd/tabletheory/templates/shared/docker-compose.yml.tmpl"
printf '{"dynamodb-local": {"image": "%s"}}\n' "${unpinned}" >"${case_dir}/scripts/local/examples-local.json"
run_guard
expect_status FAIL "template and profile JSON with a mutable tag"
expect_message "docker-compose.yml.tmpl:3" "template with a mutable tag names the line"
expect_message "examples-local.json:1" "profile JSON with a mutable tag names the line"

# 6. The same shapes pass once every reference carries the digest.
begin_case
mkdir -p "${case_dir}/scripts" "${case_dir}/examples/cdk-multilang/local" "${case_dir}/scripts/local"
printf 'services:\n  dynamodb-local:\n    image: %s\n' "${pinned}" >"${case_dir}/docker-compose.yml"
printf 'DYNAMODB_LOCAL_IMAGE ?= %s\n' "${pinned}" >"${case_dir}/Makefile"
printf '#!/usr/bin/env bash\ndocker run -d -p 8000:8000 %s -jar DynamoDBLocal.jar\n' "${pinned}" >"${case_dir}/scripts/start.sh"
printf '{"dynamodb-local": {"image": "%s"}}\n' "${pinned}" >"${case_dir}/scripts/local/examples-local.json"
cat >"${case_dir}/examples/cdk-multilang/local/run-local.sh" <<'SH'
#!/usr/bin/env bash
docker run -d -p "127.0.0.1:${PORT}:8000" ${DDB_IMAGE}
SH
run_guard
expect_status PASS "digest-pinned tree"
expect_message "pinned by digest" "digest-pinned tree"

# 7. The loopback rule survives the digest rule: a runner that publishes the
#    unauthenticated container on every interface is still rejected.
begin_case
mkdir -p "${case_dir}/examples/cdk-multilang/local"
printf '#!/usr/bin/env bash\ndocker run -d -p "${PORT}:8000" %s\n' "${pinned}" >"${case_dir}/examples/cdk-multilang/local/run-local.sh"
run_guard
expect_status FAIL "runner publishes on all interfaces"
expect_message "must publish DynamoDB Local on loopback" "runner publishes on all interfaces"

# 8. The fixture marker is the only opt-out, and it is reviewable in the file it
#    applies to: a marked file is skipped, and the unmarked tree stays clean.
begin_case
mkdir -p "${case_dir}/scripts/fixtures"
cat >"${case_dir}/scripts/fixtures/fixture.sh" <<SH
#!/usr/bin/env bash
# dynamodb-local-pin-guard: file-owns-image-fixtures
image=${unpinned}
SH
printf 'services:\n  dynamodb-local:\n    image: %s\n' "${pinned}" >"${case_dir}/docker-compose.yml"
run_guard
expect_status PASS "marked fixture file is skipped"
expect_message "pinned by digest" "marked fixture file is skipped"

# 9. A mis-rooted run fails closed instead of scanning nothing.
begin_case
rm -f "${case_dir}/go.mod"
run_guard
expect_status FAIL "root without go.mod"
expect_message "has no go.mod" "root without go.mod"

if [[ "${failures}" -ne 0 ]]; then
  echo "dynamodb-local-pin-policy: FAIL (${failures} issue(s))"
  exit 1
fi

echo "dynamodb-local-pin-policy: PASS (compose, Makefile, script, template and profile references must carry a digest; loopback rule held)"
