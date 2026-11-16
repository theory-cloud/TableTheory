#!/usr/bin/env bash
set -euo pipefail

# Verifies the CI toolchain and trigger surface (rubric COM-2):
#   - Go/Node/Python pinning, no `@latest`, pinned golangci-lint action
#   - every third-party action pinned to a full commit SHA (container actions to
#     an image digest)
#   - every `npm ci` in a workflow or script runs with `--ignore-scripts`
#   - the verify-only CI shape: a push trigger is a publish side effect only, no
#     workflow executes a gate named in the evidence registry, and the record
#     verifier runs on pull requests to staging and nowhere else
#   - the lint toolchain pin lives in this repository, because CI installs no
#     linter in a verify-only repository

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

# A push is a publish side effect, never a gate run. These are the workflows
# whose push trigger publishes something the merged state owns: the docs deploy,
# the subtree publish, and the release-please generation and publisher jobs.
# Anything else that declares a push trigger fails closed.
publish_push_workflows=(
  ".github/workflows/pages.yml"
  ".github/workflows/prerelease-pr.yml"
  ".github/workflows/prerelease.yml"
  ".github/workflows/release-pr.yml"
  ".github/workflows/release.yml"
  ".github/workflows/theorycloud-tabletheory-publish.yml"
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
      # Drop a YAML inline comment and surrounding whitespace. A SHA that
      # appears only inside a comment (e.g. `uses: owner/action@v6 # @<sha>`)
      # must not satisfy the pin check: the actual action ref is what GitHub
      # executes, and a comment carries no authority.
      ref="${ref%%#*}"
      ref="$(printf '%s' "${ref}" | sed -E 's/[[:space:]]+$//')"
      case "${ref}" in
        ./*) continue ;;
        docker://*)
          if [[ ! "${ref}" =~ @sha256:[0-9a-f]{64}$ ]]; then
            echo "${wf}: container action '${ref}' must pin an image digest"
            failures=$((failures + 1))
          fi
          continue
          ;;
      esac
      # The SHA must be the action's actual ref, not merely present on the line.
      if [[ ! "${ref}" =~ ^[^@[:space:]]+@[0-9a-f]{40}$ ]]; then
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

# Only what a workflow actually runs: inline `run:` values and the bodies of
# `run: |` / `run: >` blocks. Comments, names and `env:` values never count, so a
# workflow that documents the production command is not mistaken for one that
# runs it.
run_lines() {
  awk '
    function indent(s) { match(s, /^[ ]*/); return RLENGTH }
    {
      line = $0
      if (inblock) {
        if (line ~ /^[[:space:]]*$/) next
        if (indent(line) > blockindent) { print line; next }
        inblock = 0
      }
      if (match(line, /^[ ]*(- )?run:[ ]*/)) {
        rest = substr(line, RSTART + RLENGTH)
        sub(/[[:space:]]+$/, "", rest)
        if (rest ~ /^[|>]/) {
          inblock = 1
          blockindent = indent(line)
          next
        }
        if (rest != "") print rest
      }
    }
  ' "$1"
}

# The gate commands are read from the tracked evidence registry, so the list
# cannot drift from the policy: any command a record may execute on the
# production host fails here when a workflow tries to execute it in CI. This is
# the same classification the served MAI-4 check performs.
gate_needles() {
  python3 - <<'PY'
import json
from pathlib import Path

registry = json.loads(Path("scripts/evidence/validation-gates.json").read_text(encoding="utf-8"))
needles: list[str] = []
for _gate_id, gate in sorted((registry.get("gates") or {}).items()):
    if gate.get("kind") != "source":
        continue
    for command in gate.get("commands") or []:
        argv = [str(item) for item in command.get("argv") or []]
        if argv:
            needles.append(" ".join(argv))
for needle in dict.fromkeys(needles):
    print(needle)
PY
}

# The verify-only CI shape. Three properties, each failing closed:
#   1. a push trigger is allowed only on a publish workflow;
#   2. no workflow executes a gate command from the tracked registry;
#   3. a workflow that verifies the record runs on pull requests to staging and
#      never on a promotion pull request, where release hygiene is the only
#      allowed class.
check_verify_only_trigger_shape() {
  local wf failed_before="${failures}"
  local needles
  needles="$(gate_needles)"

  while IFS= read -r wf; do
    [[ -n "${wf}" ]] || continue

    if trigger_declared push "${wf}"; then
      if ! is_exempt "${wf}" "${publish_push_workflows[@]}"; then
        echo "${wf}: declares a push trigger but is not a publish workflow; a push carries publish side effects only, never a gate run"
        failures=$((failures + 1))
      fi
    fi

    local runs offenders
    runs="$(run_lines "${wf}")"
    if [[ -n "${runs}" && -n "${needles}" ]]; then
      offenders="$(printf '%s\n' "${runs}" | grep -F -f <(printf '%s\n' "${needles}") || true)"
      if [[ -n "${offenders}" ]]; then
        while IFS= read -r offender; do
          [[ -n "${offender}" ]] || continue
          echo "${wf}: executes a recorded gate in CI: ${offender}"
        done <<< "${offenders}"
        echo "${wf}: fix: run the gate on the production host with scripts/evidence/collect.sh, commit the record, and let CI run scripts/evidence/verify-gate.sh"
        failures=$((failures + 1))
      fi
    fi

    if printf '%s\n' "${runs}" | grep -Fq -- 'scripts/evidence/verify-gate.sh'; then
      if trigger_targets_any pull_request "${wf}" premain main; then
        echo "${wf}: verifies the evidence record on promotion pull requests; the record is verified once, on the pull request to staging, and promotion pull requests carry release hygiene only"
        failures=$((failures + 1))
      fi
    fi
  done <<< "${workflows}"

  if [[ "${failures}" -eq "${failed_before}" ]]; then
    echo "ci-verify-only-shape: PASS (push is publish-only, no gate executes in CI, verification is staging-only)"
  fi
}

# CI installs no linter: the lint gate runs on the production host, so the
# toolchain pin must live in this repository and be readable by the guard rather
# than in a workflow step the guard can no longer observe.
check_local_lint_pin() {
  local failed_before="${failures}"
  local pin

  pin="$(grep -E '^GOLANGCI_LINT_VERSION[[:space:]]*\?=' Makefile 2>/dev/null | head -n1 | sed -E 's/.*=[[:space:]]*//' || true)"
  if [[ ! "${pin}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Makefile: must declare GOLANGCI_LINT_VERSION ?= vX.Y.Z as the local lint pin"
    failures=$((failures + 1))
  fi
  if ! grep -Fq -- 'GOLANGCI_LINT_VERSION' scripts/ensure-go-lint.sh 2>/dev/null; then
    echo "scripts/ensure-go-lint.sh: must read the Makefile GOLANGCI_LINT_VERSION pin instead of restating a version"
    failures=$((failures + 1))
  fi
  if grep -Eq 'golangci-lint@v[0-9]+\.[0-9]+\.[0-9]+' scripts/ensure-go-lint.sh 2>/dev/null; then
    echo "scripts/ensure-go-lint.sh: must not hard-code a lint version; the Makefile pin is the single source"
    failures=$((failures + 1))
  fi
  local needle
  for needle in 'golangci-lint@' 'golangci/golangci-lint-action'; do
    if grep -rFq -- "${needle}" .github/workflows; then
      echo ".github/workflows: must not install a linter; verification reads a record, and the pin lives in the repository"
      failures=$((failures + 1))
    fi
  done

  if [[ "${failures}" -eq "${failed_before}" ]]; then
    echo "ci-local-lint-pin: PASS (the local pin is tracked in the repository)"
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
check_verify_only_trigger_shape
check_local_lint_pin

if [[ "${failures}" -ne 0 ]]; then
  echo "ci-toolchain: FAIL (${failures} issue(s))"
  exit 1
fi

echo "ci-toolchain: clean (toolchain ${toolchain})"
