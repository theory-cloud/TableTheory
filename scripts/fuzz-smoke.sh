#!/usr/bin/env bash
set -euo pipefail

# Bounded fuzzing pass for panic/crash detection.
#
# This verifier is intentionally short-running. It is expected to be expanded with
# targeted fuzz functions over time.

failures=0

fuzzRegex='^func[[:space:]]+Fuzz[A-Za-z0-9_]+'
missing=()

for dir in internal/expr pkg/marshal pkg/query; do
  if ! rg -n --no-heading --glob '**/*_test.go' "${fuzzRegex}" "${dir}" >/dev/null 2>&1; then
    missing+=("${dir}")
  fi
done

if [[ "${#missing[@]}" -ne 0 ]]; then
  echo "fuzz-smoke: FAIL (no fuzz targets found in: ${missing[*]})"
  echo "Add at least one 'func FuzzXxx(f *testing.F)' in each package group."
  exit 1
fi

echo "fuzz-smoke: running bounded fuzz pass (10s per package group)"

go test ./internal/expr -run '^$' -fuzz Fuzz -fuzztime=10s || failures=$((failures + 1))
go test ./pkg/marshal -run '^$' -fuzz Fuzz -fuzztime=10s || failures=$((failures + 1))
go test ./pkg/query -run '^$' -fuzz Fuzz -fuzztime=10s || failures=$((failures + 1))

if [[ "${failures}" -ne 0 ]]; then
  echo "fuzz-smoke: FAIL (${failures} package group(s) failed)"
  exit 1
fi

echo "fuzz-smoke: PASS"
