#!/usr/bin/env bash
set -euo pipefail

profile="coverage_lib.out"
targets=""

usage() {
  cat <<'EOF'
Usage:
  bash scripts/verify-coverage-packages.sh --targets <targets.tsv> [--profile <cover.out>]

Targets format:
  # comments allowed
  <package> <min_percent>

Example:
  bash scripts/coverage.sh
  bash scripts/verify-coverage-packages.sh --targets docs/development/planning/coverage-targets/theorydb-cov-2.tsv
EOF
}

while [[ $# -gt 0 ]]; do
  case "${1}" in
    --targets)
      targets="${2:-}"
      shift 2
      ;;
    --profile)
      profile="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: ${1}"
      usage
      exit 2
      ;;
  esac
done

if [[ -z "${targets}" ]]; then
  echo "missing required flag: --targets"
  usage
  exit 2
fi

if [[ ! -f "${profile}" ]]; then
  echo "missing coverage profile: ${profile}"
  echo "run: bash scripts/coverage.sh ${profile}"
  exit 1
fi

if [[ ! -f "${targets}" ]]; then
  echo "missing targets file: ${targets}"
  exit 1
fi

declare -A min_pct

while IFS= read -r line; do
  [[ -z "${line}" ]] && continue
  [[ "${line}" =~ ^# ]] && continue

  pkg="$(awk '{print $1}' <<<"${line}")"
  pct="$(awk '{print $2}' <<<"${line}")"
  if [[ -z "${pkg}" || -z "${pct}" ]]; then
    echo "invalid targets line (expected: <package> <min_percent>): ${line}"
    exit 2
  fi
  min_pct["${pkg}"]="${pct}"
done < "${targets}"

declare -A covered
declare -A total

while IFS=$'\t' read -r pkg cov tot; do
  covered["${pkg}"]="${cov}"
  total["${pkg}"]="${tot}"
done < <(
  awk '
    NR==1 { next }
    {
      k=$1
      stmts[k]=$2
      if($3>cnt[k]) cnt[k]=$3
    }
    END{
      for(k in stmts){
        split(k,a,":")
        f=a[1]
        pkg=f
        sub(/\/[^\/]+$/, "", pkg)
        total[pkg]+=stmts[k]
        if(cnt[k]>0) covered[pkg]+=stmts[k]
      }
      for(pkg in total){
        printf "%s\t%d\t%d\n", pkg, covered[pkg]+0, total[pkg]+0
      }
    }
  ' "${profile}" | sort
)

failures=0

for pkg in "${!total[@]}"; do
  if [[ -z "${min_pct[${pkg}]:-}" ]]; then
    echo "targets missing package: ${pkg}"
    failures=$((failures + 1))
  fi
done

for pkg in "${!min_pct[@]}"; do
  if [[ -z "${total[${pkg}]:-}" ]]; then
    echo "coverage missing package: ${pkg}"
    failures=$((failures + 1))
  fi
done

for pkg in "${!total[@]}"; do
  min="${min_pct[${pkg}]:-}"
  [[ -z "${min}" ]] && continue

  cov="${covered[${pkg}]}"
  tot="${total[${pkg}]}"
  pct="$(awk -v c="${cov}" -v t="${tot}" 'BEGIN { printf "%.2f", (t>0?100*c/t:0) }')"

  awk -v p="${pct}" -v m="${min}" 'BEGIN { exit !(p+0 >= m+0) }' || {
    echo "coverage too low: ${pkg} (${pct}% < ${min}%)"
    failures=$((failures + 1))
  }
done

if [[ "${failures}" -ne 0 ]]; then
  echo "coverage-packages: FAIL (${failures} issue(s))"
  exit 1
fi

echo "coverage-packages: PASS"

