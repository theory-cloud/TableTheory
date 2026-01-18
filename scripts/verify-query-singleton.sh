#!/usr/bin/env bash
set -euo pipefail

# High-risk domain rule: avoid parallel Query implementations that can drift in subtle ways over time.
#
# Pass criteria: exactly one "core query implementation" exists:
# - Either the root package has a `type query struct` implementation, OR `pkg/query` has `type Query struct`,
#   but not both. (A root-level type alias/wrapper that does not define `type query struct` is acceptable.)

failures=0

root_files="$(
  git ls-files '*.go' | rg -v '/' | rg -v '_test\.go$' | while read -r f; do
    if [[ -f "${f}" ]]; then
      echo "${f}"
    fi
  done || true
)"

set +e
root_match=""
if [[ -n "${root_files}" ]]; then
  root_match="$(rg -n --no-heading '^type\s+query\s+struct\b' ${root_files} )"
  root_status=$?
else
  root_status=1
fi
set -e
if [[ "${root_status:-1}" -gt 1 ]]; then
  echo "query-singleton: FAIL (rg error searching for root query implementation: exit ${root_status})"
  exit 1
fi

set +e
pkg_match=""
if [[ -f "pkg/query/query.go" ]]; then
  pkg_match="$(rg -n --no-heading '^type\s+Query\s+struct\b' pkg/query/query.go)"
  pkg_status=$?
else
  pkg_status=1
fi
set -e
if [[ "${pkg_status}" -gt 1 ]]; then
  echo "query-singleton: FAIL (rg error searching for pkg/query implementation: exit ${pkg_status})"
  exit 1
fi

root_present=0
pkg_present=0
if [[ "${root_status:-1}" -eq 0 ]]; then
  root_present=1
fi
if [[ "${pkg_status}" -eq 0 ]]; then
  pkg_present=1
fi

if [[ $((root_present + pkg_present)) -ne 1 ]]; then
  echo "query-singleton: FAIL (expected exactly one Query implementation; found root=${root_present} pkg/query=${pkg_present})"
  if [[ "${root_present}" -eq 1 ]]; then
    echo "query-singleton: root implementation detected:"
    echo "${root_match}"
  fi
  if [[ "${pkg_present}" -eq 1 ]]; then
    echo "query-singleton: pkg/query implementation detected:"
    echo "${pkg_match}"
  fi
  echo "query-singleton: see docs/development/planning/theorydb-maintainability-roadmap.md"
  exit 1
fi

echo "query-singleton: PASS"
