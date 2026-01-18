#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f go.mod ]]; then
  echo "go.mod not found at repo root"
  exit 1
fi

toolchain="$(awk '/^toolchain / {print $2}' go.mod | head -n 1)"
if [[ -z "${toolchain}" ]]; then
  echo "go.mod missing 'toolchain' directive (required for reproducibility)"
  exit 1
fi

workflows="$(find .github/workflows -maxdepth 1 -type f -name '*.yml' -o -name '*.yaml' 2>/dev/null | sort || true)"
if [[ -z "${workflows}" ]]; then
  echo "no workflows found under .github/workflows"
  exit 1
fi

failures=0

while IFS= read -r wf; do
  if grep -Eq '^[[:space:]]*uses:[[:space:]]*actions/setup-go@' "${wf}"; then
    grep -Ev '^[[:space:]]*#' "${wf}" | grep -q 'go-version-file: go.mod' || {
      echo "${wf}: setup-go must use go-version-file: go.mod"
      failures=$((failures + 1))
    }
  fi

  if grep -Eq '^[[:space:]]*uses:[[:space:]]*actions/setup-node@' "${wf}"; then
    grep -Ev '^[[:space:]]*#' "${wf}" | grep -Eq 'node-version:[[:space:]]*["'"'"']?24(\\.x)?["'"'"']?' || {
      echo "${wf}: setup-node must pin node-version: 24"
      failures=$((failures + 1))
    }
    if grep -Ev '^[[:space:]]*#' "${wf}" | grep -Eq 'node-version:[[:space:]]*latest'; then
      echo "${wf}: setup-node node-version must not be 'latest'"
      failures=$((failures + 1))
    fi
  fi

  if grep -Eq '^[[:space:]]*uses:[[:space:]]*actions/setup-python@' "${wf}"; then
    grep -Ev '^[[:space:]]*#' "${wf}" | grep -Eq 'python-version:[[:space:]]*"?3[.]14([.]x)?"?' || {
      echo "${wf}: setup-python must pin python-version: 3.14"
      failures=$((failures + 1))
    }
    if grep -Ev '^[[:space:]]*#' "${wf}" | grep -Eq 'python-version:[[:space:]]*latest'; then
      echo "${wf}: setup-python python-version must not be 'latest'"
      failures=$((failures + 1))
    fi
  fi

  # Reject @latest in workflows to avoid silent behavior drift.
  if grep -Ev '^[[:space:]]*#' "${wf}" | grep -Eq '@latest'; then
    echo "${wf}: contains @latest; pin versions"
    failures=$((failures + 1))
  fi

  if grep -q 'golangci/golangci-lint-action' "${wf}"; then
    grep -Eq 'version:[[:space:]]*v[0-9]+' "${wf}" || {
      echo "${wf}: golangci-lint-action must pin version: vX.Y.Z"
      failures=$((failures + 1))
    }
    if grep -Eq 'version:[[:space:]]*latest' "${wf}"; then
      echo "${wf}: golangci-lint-action version must not be 'latest'"
      failures=$((failures + 1))
    fi
  fi
done <<< "${workflows}"

if [[ "${failures}" -ne 0 ]]; then
  echo "ci-toolchain: FAIL (${failures} issue(s))"
  exit 1
fi

echo "ci-toolchain: clean (toolchain ${toolchain})"
