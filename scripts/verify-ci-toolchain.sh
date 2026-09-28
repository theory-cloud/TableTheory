#!/usr/bin/env bash
set -euo pipefail

# Verifies the CI toolchain and trigger surface (rubric COM-2):
#   - Go/Node/Python pinning, no `@latest`, pinned golangci-lint action
#   - every third-party action pinned to a full commit SHA (container actions to
#     an image digest)
#   - every `npm ci` in a workflow or script runs with `--ignore-scripts`
#   - R-F1 trigger parity: a job that runs on a push to `staging`, or on a
#     promotion pull request (base `premain`/`main`), must also be exercised on
#     pull requests to `staging`, so a PR cannot be green while its promotion
#     fails

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

# --- helpers: action pinning, lockfile-install hygiene, trigger parity -------

# Extract the branch list declared under `<key>:` inside the workflow's `on:`
# block (inline `branches: [a, b]` or a block list). Prints one entry per line
# and nothing when the event declares no branch filter.
trigger_branches() {
  local key="$1" wf="$2"
  awk -v key="${key}" '
    /^[^[:space:]]/ {
      in_on = ($0 ~ /^on:[[:space:]]*($|#)/)
      in_sub = 0
      in_branches = 0
    }
    in_on && /^[[:space:]]{2}[^[:space:]]/ {
      in_sub = ($0 ~ "^[[:space:]]{2}" key ":[[:space:]]*($|#)")
      in_branches = 0
      next
    }
    in_on && in_sub && /^[[:space:]]{4}branches:[[:space:]]*/ {
      rest = $0
      sub(/^[[:space:]]{4}branches:[[:space:]]*/, "", rest)
      if (rest ~ /^($|#)/) {
        in_branches = 1
      } else {
        print rest
        in_branches = 0
      }
      next
    }
    in_on && in_sub && in_branches && /^[[:space:]]{6}-/ {
      rest = $0
      sub(/^[[:space:]]{6}-[[:space:]]*/, "", rest)
      print rest
      next
    }
    in_on && in_sub && in_branches { in_branches = 0 }
  ' "${wf}"
}

# True when the `on:` block declares `<key>:` as an event. Flow-style empty
# bodies (`pull_request: {}`) count as declared; a declared event with no branch
# filter means "every branch". Only block-style `on:`/`branches:` forms are
# parsed, which is what every workflow in this repository uses.
trigger_declared() {
  local key="$1" wf="$2"
  awk -v key="${key}" '
    /^[^[:space:]]/ { in_on = ($0 ~ /^on:[[:space:]]*($|#)/) }
    in_on && $0 ~ "^[[:space:]]{2}" key ":[[:space:]]*($|#|\\{|\\[)" { found = 1 }
    END { exit found ? 0 : 1 }
  ' "${wf}"
}

# True when the event's branch filter lets <branch> through. An absent branch
# filter means "every branch".
trigger_covers_branch() {
  local key="$1" branch="$2" wf="$3"
  local branches
  branches="$(trigger_branches "${key}" "${wf}")"
  if [[ -z "${branches//[[:space:]]/}" ]]; then
    trigger_declared "${key}" "${wf}"
    return
  fi
  grep -qw -- "${branch}" <<< "${branches}"
}

# True when the event declares a branch filter naming any of the arguments.
trigger_targets_any() {
  local key="$1" wf="$2"
  shift 2
  local branches
  branches="$(trigger_branches "${key}" "${wf}")"
  [[ -n "${branches//[[:space:]]/}" ]] || return 1
  local wanted
  for wanted in "$@"; do
    if grep -qw -- "${wanted}" <<< "${branches}"; then
      return 0
    fi
  done
  return 1
}

# R-F1: a workflow that runs on a push to `staging`, or on a pull request whose
# base is `premain`/`main`, must also be exercised on pull requests to
# `staging`. Otherwise a PR merges green and then fails the very job its
# promotion depends on (the #623 failure mode).
#
# Exemptions are explicit and each must be justified in the PR body:
#   - release-hygiene.yml IS the promotion-lane gate. Release policy runs
#     release hygiene, not the full rubric, on premain/main PRs, so requiring
#     it on staging PRs would duplicate the rubric lane.
# An empty exemption list means "no workflow may skip staging-PR coverage".
exempt_from_staging_pr_coverage=(
)
exempt_from_promotion_pr_coverage=(
  ".github/workflows/release-hygiene.yml"
)

is_exempt() {
  local candidate="$1"
  shift
  local entry
  for entry in "$@"; do
    [[ "${candidate}" == "${entry}" ]] && return 0
  done
  return 1
}

# R-G2: every third-party action must be pinned to a full commit SHA (and every
# container action to an image digest). A mutable tag ref silently changes what
# a protected-branch gate executes.
check_action_pins() {
  local wf line ref
  while IFS= read -r wf; do
    [[ -n "${wf}" ]] || continue
    while IFS= read -r line; do
      [[ -n "${line}" ]] || continue
      ref="$(sed -E 's/^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]*//' <<< "${line}")"
      case "${ref}" in
        ./*) continue ;;
        docker://*)
          if [[ ! "${ref}" =~ @sha256:[0-9a-f]{64} ]]; then
            echo "${wf}: container action '${ref}' must pin an image digest"
            failures=$((failures + 1))
          fi
          continue
          ;;
      esac
      if [[ ! "${ref}" =~ @[0-9a-f]{40}([[:space:]]|$) ]]; then
        echo "${wf}: action reference '${ref}' is not pinned to a full commit SHA"
        failures=$((failures + 1))
      fi
    done < <(grep -E '^[[:space:]]*(-[[:space:]]+)?uses:' "${wf}" || true)
  done <<< "${workflows}"
}

# Print `file:line:...` for every `npm ci` invocation that omits
# `--ignore-scripts`. Prose/echo lines are skipped so the checker's own failure
# messages cannot match themselves.
npm_ci_offenders() {
  local file="$1"
  grep -nE '(^|[[:space:]]|!|&&|\|\||;|\()[[:space:]]*npm([[:space:]]+[^[:space:]]+)*[[:space:]]+ci([[:space:]]|$)' "${file}" 2>/dev/null \
    | grep -vE '^[0-9]+:[[:space:]]*(#|echo|printf)' \
    | grep -v -- '--ignore-scripts' || true
}

# R-G3: lockfile installs must run with lifecycle scripts disabled. A
# dependency install script would otherwise execute with the gate's privileges.
check_npm_ci_across_repo() {
  local file offenders offender scanned=0
  while IFS= read -r file; do
    [[ -n "${file}" ]] || continue
    # Policy tests deliberately embed offending `npm ci` fixtures for the guard
    # to reject. They declare that with this marker so the guard does not match
    # its own fixtures (the marker is a reviewable, greppable opt-out).
    if grep -Fq 'ci-toolchain-guard: file-owns-npm-ci-fixtures' "${file}"; then
      continue
    fi
    scanned=$((scanned + 1))
    offenders="$(npm_ci_offenders "${file}")"
    [[ -n "${offenders}" ]] || continue
    while IFS= read -r offender; do
      [[ -n "${offender}" ]] || continue
      echo "${file}:${offender} must pass --ignore-scripts (lockfile installs run with lifecycle scripts disabled)"
    done <<< "${offenders}"
    failures=$((failures + 1))
  done < <(find .github/workflows scripts examples contract-tests gov-infra \
    -type f \( -name '*.yml' -o -name '*.yaml' -o -name '*.sh' \) \
    -not -path '*/node_modules/*' 2>/dev/null | sort)

  if [[ "${scanned}" -eq 0 ]]; then
    echo "ci-supply-chain: FAIL (no workflow/script files found to scan)"
    failures=$((failures + 1))
  fi
}

check_trigger_parity() {
  local wf failed_before="${failures}"

  while IFS= read -r wf; do
    [[ -n "${wf}" ]] || continue

    local pr_covers_staging=false
    if trigger_covers_branch pull_request staging "${wf}"; then
      pr_covers_staging=true
    fi

    if trigger_covers_branch push staging "${wf}" && [[ "${pr_covers_staging}" != true ]]; then
      if ! is_exempt "${wf}" "${exempt_from_staging_pr_coverage[@]}"; then
        echo "${wf}: runs on push to staging but is never exercised on pull requests to staging; add a staging pull_request trigger or record a justified exemption"
        failures=$((failures + 1))
      fi
    fi

    if trigger_targets_any pull_request "${wf}" premain main && [[ "${pr_covers_staging}" != true ]]; then
      if ! is_exempt "${wf}" "${exempt_from_promotion_pr_coverage[@]}"; then
        echo "${wf}: gates pull requests to premain/main but not pull requests to staging; a staging PR could merge green and then fail this job on promotion"
        failures=$((failures + 1))
      fi
    fi
  done <<< "${workflows}"

  if [[ "${failures}" -eq "${failed_before}" ]]; then
    echo "ci-trigger-parity: PASS (every push/promotion job is exercised on staging PRs)"
  fi
}

while IFS= read -r wf; do
  uncommented="$(grep -Ev '^[[:space:]]*#' "${wf}" || true)"

  if grep -Eq '^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]*actions/setup-go@' "${wf}"; then
    grep -q 'go-version-file: go.mod' <<< "${uncommented}" || {
      echo "${wf}: setup-go must use go-version-file: go.mod"
      failures=$((failures + 1))
    }
  fi

  if grep -Eq '^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]*actions/setup-node@' "${wf}"; then
    node_direct_pin=false
    node_matrix_pin=false
    if grep -Eq "node-version:[[:space:]]*\"?24(\\.x)?\"?" <<< "${uncommented}"; then
      node_direct_pin=true
    fi
    if grep -Eq "node-version:[[:space:]]*\\$\\{\\{[[:space:]]*matrix[.]node-version[[:space:]]*\\}\\}" <<< "${uncommented}" \
      && grep -Eq "node-version:[[:space:]]*\\[[^]]*\"?24(\\.x)?\"?[^]]*\\]" <<< "${uncommented}"; then
      node_matrix_pin=true
    fi
    if [[ "${node_direct_pin}" != true && "${node_matrix_pin}" != true ]]; then
      echo "${wf}: setup-node must pin node-version: 24 or use a matrix that includes 24"
      failures=$((failures + 1))
    fi
    if grep -Eq 'node-version:[[:space:]]*latest|node-version:[[:space:]]*\[[^]]*latest' <<< "${uncommented}"; then
      echo "${wf}: setup-node node-version must not be 'latest'"
      failures=$((failures + 1))
    fi
  fi

  if grep -Eq '^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]*actions/setup-python@' "${wf}"; then
    python_direct_pin=false
    python_matrix_pin=false
    if grep -Eq "python-version:[[:space:]]*\"?3[.]14([.]x)?\"?" <<< "${uncommented}"; then
      python_direct_pin=true
    fi
    if grep -Eq "python-version:[[:space:]]*\\$\\{\\{[[:space:]]*matrix[.]python-version[[:space:]]*\\}\\}" <<< "${uncommented}" \
      && grep -Eq "python-version:[[:space:]]*\\[[^]]*\"?3[.]14([.]x)?\"?[^]]*\\]" <<< "${uncommented}"; then
      python_matrix_pin=true
    fi
    if [[ "${python_direct_pin}" != true && "${python_matrix_pin}" != true ]]; then
      echo "${wf}: setup-python must pin python-version: 3.14 or use a matrix that includes 3.14"
      failures=$((failures + 1))
    fi
    if grep -Eq 'python-version:[[:space:]]*latest|python-version:[[:space:]]*\[[^]]*latest' <<< "${uncommented}"; then
      echo "${wf}: setup-python python-version must not be 'latest'"
      failures=$((failures + 1))
    fi
  fi

  # Reject @latest in workflows to avoid silent behavior drift.
  if grep -Eq '@latest' <<< "${uncommented}"; then
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

check_action_pins
check_npm_ci_across_repo
check_trigger_parity

if [[ "${failures}" -ne 0 ]]; then
  echo "ci-toolchain: FAIL (${failures} issue(s))"
  exit 1
fi

echo "ci-toolchain: clean (toolchain ${toolchain})"
