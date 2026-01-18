#!/usr/bin/env bash
set -euo pipefail

required=(
  "docs/development/planning/theorydb-controls-matrix.md"
  "docs/development/planning/theorydb-10of10-rubric.md"
  "docs/development/planning/theorydb-10of10-roadmap.md"
  "docs/development/planning/theorydb-branch-release-policy.md"
  "docs/development/planning/theorydb-encryption-tag-roadmap.md"
  "docs/development/planning/theorydb-maintainability-roadmap.md"
  "docs/development/planning/theorydb-evidence-plan.md"
  "docs/development/planning/theorydb-threat-model.md"
)

failures=0

for f in "${required[@]}"; do
  if [[ ! -f "${f}" ]]; then
    echo "missing: ${f}"
    failures=$((failures + 1))
  fi
done

if [[ -f "docs/development/planning/theorydb-10of10-rubric.md" ]]; then
  grep -Eq '^-\s+\*\*Rubric version:\*\*\s+`v[0-9]+' docs/development/planning/theorydb-10of10-rubric.md || {
    echo "theorydb-10of10-rubric.md: missing rubric version line"
    failures=$((failures + 1))
  }
fi

if [[ "${failures}" -ne 0 ]]; then
  echo "planning-docs: FAIL (${failures} issue(s))"
  exit 1
fi

echo "planning-docs: present"
