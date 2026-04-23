#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

usage() {
  cat <<EOF_USAGE
Usage:
  bash scripts/sync_theorycloud_tabletheory_subtree.sh [--stage STAGE] [--source-s3-uri URI] [--output DIR]

Environment:
  THEORYCLOUD_STAGE                           Stage name or branch name. Accepts: lab, live, premain, main
  THEORYCLOUD_TABLETHEORY_SUBTREE_OUTPUT_DIR  Staging root directory. Default: /tmp/theorycloud-tabletheory-source
  THEORYCLOUD_TABLETHEORY_SOURCE_S3_URI       Optional override for the subtree destination S3 URI
  KT_SOURCE_S3_URI                            Alternate override for the subtree destination S3 URI
  THEORYCLOUD_S3_SYNC_DELETE                  Default: true. When true, prune objects under theorycloud/tabletheory/
  THEORYCLOUD_S3_SYNC_DRY_RUN                 Default: false. When true, print the sync plan without calling AWS
EOF_USAGE
}

fail() {
  echo "sync-theorycloud-tabletheory-subtree: FAIL ($*)" >&2
  exit 1
}

normalize_stage() {
  local value="${1:-}"
  value="${value,,}"
  case "${value}" in
    lab|live)
      printf '%s\n' "${value}"
      ;;
    premain|refs/heads/premain)
      printf '%s\n' 'lab'
      ;;
    main|refs/heads/main)
      printf '%s\n' 'live'
      ;;
    *)
      return 1
      ;;
  esac
}

current_ref_name() {
  if [[ -n "${GITHUB_REF_NAME:-}" ]]; then
    printf '%s\n' "${GITHUB_REF_NAME}"
    return 0
  fi
  if [[ -n "${GITHUB_REF:-}" ]]; then
    printf '%s\n' "${GITHUB_REF#refs/heads/}"
    return 0
  fi
  git -C "${REPO_ROOT}" branch --show-current 2>/dev/null || true
}

resolve_stage() {
  local input="${1:-}"
  local resolved=""

  if [[ -n "${input}" ]]; then
    resolved="$(normalize_stage "${input}" || true)"
    if [[ -z "${resolved}" ]]; then
      fail "unsupported stage or branch '${input}'; expected lab|live or premain|main"
    fi
    printf '%s\n' "${resolved}"
    return 0
  fi

  local ref_name
  ref_name="$(current_ref_name)"
  resolved="$(normalize_stage "${ref_name}" || true)"
  if [[ -z "${resolved}" ]]; then
    fail "unable to resolve stage from current ref '${ref_name:-<empty>}'; set THEORYCLOUD_STAGE to lab|live or run on premain/main"
  fi
  printf '%s\n' "${resolved}"
}

default_source_s3_uri_for_stage() {
  local stage="$1"
  case "${stage}" in
    lab) printf '%s\n' 's3://kt-sources-lab-787107040121/theorycloud/tabletheory/' ;;
    live) printf '%s\n' 's3://kt-sources-live-787107040121/theorycloud/tabletheory/' ;;
    *) return 1 ;;
  esac
}

require_source_s3_uri() {
  local value="$1"
  local label="$2"
  if [[ -z "${value}" ]]; then
    fail "missing ${label}"
  fi
  if [[ ! "${value}" =~ ^s3://[^/]+/theorycloud/tabletheory/?$ ]]; then
    fail "${label} must point exactly to s3://<bucket>/theorycloud/tabletheory/: ${value}"
  fi
}

STAGE_INPUT="${THEORYCLOUD_STAGE:-}"
OUTPUT_DIR="${THEORYCLOUD_TABLETHEORY_SUBTREE_OUTPUT_DIR:-/tmp/theorycloud-tabletheory-source}"
SOURCE_S3_URI="${THEORYCLOUD_TABLETHEORY_SOURCE_S3_URI:-${KT_SOURCE_S3_URI:-}}"
SYNC_DELETE="${THEORYCLOUD_S3_SYNC_DELETE:-true}"
SYNC_DRY_RUN="${THEORYCLOUD_S3_SYNC_DRY_RUN:-false}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --stage)
      STAGE_INPUT="$2"
      shift 2
      ;;
    --source-s3-uri)
      SOURCE_S3_URI="$2"
      shift 2
      ;;
    --output)
      OUTPUT_DIR="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

STAGE="$(resolve_stage "${STAGE_INPUT}")"
if [[ -z "${SOURCE_S3_URI}" ]]; then
  SOURCE_S3_URI="$(default_source_s3_uri_for_stage "${STAGE}" || true)"
fi
require_source_s3_uri "${SOURCE_S3_URI}" "THEORYCLOUD_TABLETHEORY_SOURCE_S3_URI"
SOURCE_S3_URI="${SOURCE_S3_URI%/}/"

bash "${SCRIPT_DIR}/stage_theorycloud_tabletheory_subtree.sh" --output "${OUTPUT_DIR}"

SUBTREE_DIR="${OUTPUT_DIR%/}/tabletheory"
if [[ ! -d "${SUBTREE_DIR}" ]]; then
  fail "missing staged subtree at ${SUBTREE_DIR}; staging helper did not produce tabletheory/"
fi
if [[ ! -f "${SUBTREE_DIR}/source-manifest.json" ]]; then
  fail "missing staged provenance manifest at ${SUBTREE_DIR}/source-manifest.json"
fi

sync_flags=()
sync_flags+=(--no-follow-symlinks)
if [[ "${SYNC_DELETE,,}" == "true" ]]; then
  sync_flags+=(--delete)
fi

if [[ "${SYNC_DRY_RUN,,}" == "true" ]]; then
  echo "sync-theorycloud-tabletheory-subtree: DRY RUN"
  echo "stage=${STAGE}"
  echo "source=${SUBTREE_DIR}/"
  echo "destination=${SOURCE_S3_URI}"
  if [[ "${SYNC_DELETE,,}" == "true" ]]; then
    echo "delete=true"
  else
    echo "delete=false"
  fi
  echo "command=aws s3 sync ${SUBTREE_DIR}/ ${SOURCE_S3_URI} ${sync_flags[*]:-}"
  echo "sync-theorycloud-tabletheory-subtree: PASS (dry-run; target=${SOURCE_S3_URI})"
  exit 0
fi

command -v aws >/dev/null 2>&1 || fail "aws CLI is required"

echo "syncing TableTheory subtree to ${SOURCE_S3_URI}"
if ! aws s3 sync "${SUBTREE_DIR}/" "${SOURCE_S3_URI}" "${sync_flags[@]}"; then
  fail "aws s3 sync failed for ${SOURCE_S3_URI}"
fi

echo "sync-theorycloud-tabletheory-subtree: PASS (target=${SOURCE_S3_URI})"
