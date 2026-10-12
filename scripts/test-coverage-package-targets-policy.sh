#!/usr/bin/env bash
set -euo pipefail

# TTSEC2-M4-T4 focused policy test for
# finding commit:033ccfd523c88191800e53c009c45d73.
#
# `go test` runs every package with a shared `-coverpkg` set, so Go emits the
# same source segment once per test binary: covered in one record and uncovered
# in the others. The package-floor mode of `scripts/verify-coverage.sh` must
# aggregate by unique source segment (each segment's statements counted once, and
# covered when any record has a positive count) exactly like `go tool cover`.
# Summing every record instead reports a heavily covered package as a fraction of
# the duplicate count and produces false failures.
#
# This test builds a real two-package module, generates duplicated and unique
# profiles for `set`, `count`, and `atomic` modes, and compares the verifier's
# per-package result to the `go tool cover` oracle: a floor just below the oracle
# must pass and a floor just above it must fail, which brackets the verifier's
# computed percentage to the oracle within rounding tolerance.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${REPO_ROOT}"

workdir="$(mktemp -d)"
trap 'rm -rf "${workdir}"' EXIT

module="example.com/ttcov"
mkdir -p "${workdir}/a" "${workdir}/b"

cat > "${workdir}/go.mod" <<EOF
module ${module}

go 1.24
EOF

# Package a: fully covered.
cat > "${workdir}/a/a.go" <<'EOF'
package a

func Fully(x int) int {
	if x > 0 {
		return x + 1
	}
	return 0
}
EOF

cat > "${workdir}/a/a_test.go" <<'EOF'
package a

import "testing"

func TestFully(t *testing.T) {
	if Fully(1) != 2 {
		t.Fatal("bad")
	}
	if Fully(-1) != 0 {
		t.Fatal("bad")
	}
}
EOF

# Package b: only the first branch is exercised, so it is partially covered.
cat > "${workdir}/b/b.go" <<'EOF'
package b

func Partial(x int) int {
	if x > 0 {
		return x + 1
	}
	if x < -100 {
		return -1
	}
	return 0
}
EOF

cat > "${workdir}/b/b_test.go" <<'EOF'
package b

import "testing"

func TestPartial(t *testing.T) {
	if Partial(1) != 2 {
		t.Fatal("bad")
	}
	if Partial(0) != 0 {
		t.Fatal("bad")
	}
}
EOF

go_tool_cover_pct() {
  ( cd "${workdir}" && go tool cover -func="$1" ) | tail -n 1 | awk '{gsub(/%$/, "", $NF); print $NF}'
}

floor_below() {
  python3 -c 'import sys; print(f"{float(sys.argv[1]) - 0.5:.2f}")' "$1"
}

floor_above() {
  python3 -c 'import sys; print(f"{float(sys.argv[1]) + 0.5:.2f}")' "$1"
}

run_verifier() {
  local profile="$1" targets="$2"
  set +e
  bash scripts/verify-coverage.sh --profile "${profile}" --package-targets "${targets}"
  local status=$?
  set -e
  return "${status}"
}

failures=0

for mode in set count atomic; do
  echo "coverage-packages-policy: mode=${mode}"

  ( cd "${workdir}" && go test -count=1 -covermode="${mode}" -coverprofile="unique_a.out" ./a >/dev/null )
  ( cd "${workdir}" && go test -count=1 -covermode="${mode}" -coverprofile="unique_b.out" ./b >/dev/null )
  ( cd "${workdir}" && go test -count=1 -covermode="${mode}" -coverpkg=./... -coverprofile="merged.out" ./... >/dev/null )

  pct_a="$(go_tool_cover_pct "${workdir}/unique_a.out")"
  pct_b="$(go_tool_cover_pct "${workdir}/unique_b.out")"

  # A unique, non-duplicated profile: concatenate the per-package profiles under
  # one header, like a single-binary run that instruments only its own package.
  {
    head -n 1 "${workdir}/unique_a.out"
    tail -n +2 "${workdir}/unique_a.out"
    tail -n +2 "${workdir}/unique_b.out"
  } > "${workdir}/unique.out"

  merged_pct_a="$(grep -c "/a/a.go:" "${workdir}/merged.out" || true)"
  merged_pct_b="$(grep -c "/b/b.go:" "${workdir}/merged.out" || true)"
  if ((merged_pct_a < 2 || merged_pct_b < 2)); then
    echo "coverage-packages-policy: FAIL (${mode}: merged profile does not duplicate source segments)"
    failures=$((failures + 1))
  fi

  # Non-vacuity: the naive per-row sum must disagree with the oracle, otherwise
  # this fixture would not exercise the duplicate-segment defect.
  naive_a="$(python3 - "${workdir}/merged.out" "${module}/a" <<'PY'
import sys
from pathlib import Path

profile, pkg_prefix = sys.argv[1], sys.argv[2]
covered = total = 0
for raw in Path(profile).read_text().splitlines()[1:]:
    parts = raw.split()
    if len(parts) < 3:
        continue
    location, stmts_s, count_s = parts[:3]
    if not location.startswith(pkg_prefix + "/"):
        continue
    stmts = int(float(stmts_s))
    total += stmts
    if int(float(count_s)) > 0:
        covered += stmts
print(f"{100.0 * covered / total:.2f}" if total else "0.00")
PY
)"
  if python3 -c 'import sys; sys.exit(0 if abs(float(sys.argv[1]) - float(sys.argv[2])) < 0.05 else 1)' "${naive_a}" "${pct_a}"; then
    echo "coverage-packages-policy: FAIL (${mode}: duplicated fixture does not expose the double-counting defect)"
    failures=$((failures + 1))
  fi

  for fixture in merged unique; do
    profile="${workdir}/${fixture}.out"

    {
      printf '%s %s\n' "${module}/a" "$(floor_below "${pct_a}")"
      printf '%s %s\n' "${module}/b" "$(floor_below "${pct_b}")"
    } > "${workdir}/targets_pass.tsv"
    if ! run_verifier "${profile}" "${workdir}/targets_pass.tsv" >/dev/null; then
      echo "coverage-packages-policy: FAIL (${mode}/${fixture}: verifier below oracle floor, oracle a=${pct_a}% b=${pct_b}%)"
      failures=$((failures + 1))
    fi

    {
      printf '%s %s\n' "${module}/a" "$(floor_above "${pct_a}")"
      printf '%s %s\n' "${module}/b" "$(floor_above "${pct_b}")"
    } > "${workdir}/targets_fail.tsv"
    if run_verifier "${profile}" "${workdir}/targets_fail.tsv" >/dev/null 2>&1; then
      echo "coverage-packages-policy: FAIL (${mode}/${fixture}: verifier above oracle floor, oracle a=${pct_a}% b=${pct_b}%)"
      failures=$((failures + 1))
    fi
  done
done

if [[ "${failures}" -ne 0 ]]; then
  echo "coverage-packages-policy: FAIL (${failures} issue(s))"
  exit 1
fi

echo "coverage-packages-policy: PASS"
