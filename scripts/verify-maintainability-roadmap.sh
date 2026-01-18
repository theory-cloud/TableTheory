#!/usr/bin/env bash
set -euo pipefail

roadmap="docs/development/planning/theorydb-maintainability-roadmap.md"

if [[ ! -f "${roadmap}" ]]; then
  echo "maintainability-roadmap: FAIL (missing ${roadmap})"
  exit 1
fi

failures=0

grep -q '^## Baseline' "${roadmap}" || { echo "maintainability-roadmap: missing '## Baseline' section"; failures=$((failures + 1)); }
grep -q '^## Workstreams' "${roadmap}" || { echo "maintainability-roadmap: missing '## Workstreams' section"; failures=$((failures + 1)); }
grep -q '^## Milestones' "${roadmap}" || { echo "maintainability-roadmap: missing '## Milestones' section"; failures=$((failures + 1)); }

grep -q 'MAI-1' "${roadmap}" || { echo "maintainability-roadmap: missing MAI-1 reference"; failures=$((failures + 1)); }
grep -q 'MAI-2' "${roadmap}" || { echo "maintainability-roadmap: missing MAI-2 reference"; failures=$((failures + 1)); }
grep -q 'MAI-3' "${roadmap}" || { echo "maintainability-roadmap: missing MAI-3 reference"; failures=$((failures + 1)); }

grep -q 'Snapshot (' "${roadmap}" || { echo "maintainability-roadmap: missing baseline snapshot marker (e.g., 'Snapshot (YYYY-MM-DD)')"; failures=$((failures + 1)); }

if [[ "${failures}" -ne 0 ]]; then
  echo "maintainability-roadmap: FAIL (${failures} issue(s))"
  exit 1
fi

echo "maintainability-roadmap: PASS"

