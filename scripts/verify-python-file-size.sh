#!/usr/bin/env bash
set -euo pipefail

# High-risk domain rule: large, monolithic files are a reliability and security risk because they increase review
# surface area and make semantic drift more likely over time.
#
# This verifier enforces a strict line-count budget for production Python files.

max_lines=1500

failures=0

files="$(git ls-files 'py/src/**/*.py' 'py/src/*.py' 2>/dev/null || true)"
if [[ -z "${files}" ]]; then
  echo "py-file-size: SKIP (no production Python files found)"
  exit 0
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
    echo "py-file-size: ${f}: ${lines} lines (max ${max_lines})"
    failures=$((failures + 1))
  fi
done <<< "${files}"

if [[ "${failures}" -ne 0 ]]; then
  echo "py-file-size: FAIL (${failures} file(s) exceed ${max_lines} lines)"
  echo "py-file-size: see docs/development/planning/theorydb-maintainability-roadmap.md"
  exit 1
fi

echo "py-file-size: PASS (max ${max_lines})"

