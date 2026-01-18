#!/usr/bin/env bash
set -euo pipefail

# High-risk domain rule: large, monolithic files are a reliability and security risk because they increase review
# surface area and make semantic drift more likely over time.
#
# This verifier enforces a strict line-count budget for production Go files.

max_lines=2500

failures=0

files="$(git ls-files '*.go' | rg -v '(^|/)examples/|(^|/)tests/|(^|/)scripts/|(^|/)pkg/(mocks|testing)/|_test\.go$' || true)"
if [[ -z "${files}" ]]; then
  echo "go-file-size: FAIL (no production Go files found)"
  exit 1
fi

while IFS= read -r f; do
  if [[ -z "${f}" ]]; then
    continue
  fi
  if [[ ! -f "${f}" ]]; then
    continue
  fi
  lines="$(wc -l <"${f}" | tr -d ' ')"
  if [[ "${lines}" -gt "${max_lines}" ]]; then
    echo "go-file-size: ${f}: ${lines} lines (max ${max_lines})"
    failures=$((failures + 1))
  fi
done <<< "${files}"

if [[ "${failures}" -ne 0 ]]; then
  echo "go-file-size: FAIL (${failures} file(s) exceed ${max_lines} lines)"
  echo "go-file-size: see docs/development/planning/theorydb-maintainability-roadmap.md"
  exit 1
fi

echo "go-file-size: PASS (max ${max_lines})"
