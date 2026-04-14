#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

usage() {
  cat <<EOF_USAGE
Usage:
  bash scripts/trigger_theorycloud_publish.sh [--stage STAGE] [--publish-url URL] [--source-revision SHA] [--idempotency-key KEY] [--reason TEXT] [--force]

Environment:
  THEORYCLOUD_STAGE           Stage name or branch name. Accepts: lab, live, premain, main
  THEORYCLOUD_PUBLISH_URL     Optional override for the publish endpoint URL
  KT_PUBLISH_URL              Alternate override for the publish endpoint URL
  THEORYCLOUD_PUBLISH_DRY_RUN Default: false. When true, print the request instead of invoking KT
  THEORYCLOUD_PUBLISH_REASON  Default: docs sync complete
  THEORYCLOUD_PUBLISH_FORCE   Default: false
  SOURCE_REVISION             Optional source revision override
  AWS_REGION                  Default: us-east-1
EOF_USAGE
}

fail() {
  echo "trigger-theorycloud-publish: FAIL ($*)" >&2
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

default_publish_url_for_stage() {
  local stage="$1"
  case "${stage}" in
    lab) printf '%s\n' 'https://l0lw87lsp1.execute-api.us-east-1.amazonaws.com/v1/internal/publish/theorycloud' ;;
    live) printf '%s\n' 'https://at3k47vix3.execute-api.us-east-1.amazonaws.com/v1/internal/publish/theorycloud' ;;
    *) return 1 ;;
  esac
}

require_publish_url() {
  local value="$1"
  if [[ -z "${value}" ]]; then
    fail "missing THEORYCLOUD_PUBLISH_URL"
  fi
  if [[ ! "${value}" =~ ^https://[^[:space:]]+/v1/internal/publish/theorycloud/?$ ]]; then
    fail "publish URL must point exactly to https://<host>/v1/internal/publish/theorycloud: ${value}"
  fi
}

STAGE_INPUT="${THEORYCLOUD_STAGE:-}"
PUBLISH_URL="${THEORYCLOUD_PUBLISH_URL:-${KT_PUBLISH_URL:-}}"
SOURCE_REVISION="${SOURCE_REVISION:-}"
IDEMPOTENCY_KEY=""
REASON="${THEORYCLOUD_PUBLISH_REASON:-docs sync complete}"
FORCE="${THEORYCLOUD_PUBLISH_FORCE:-false}"
PUBLISH_DRY_RUN="${THEORYCLOUD_PUBLISH_DRY_RUN:-false}"
AWS_REGION="${AWS_REGION:-us-east-1}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --stage)
      STAGE_INPUT="$2"
      shift 2
      ;;
    --publish-url)
      PUBLISH_URL="$2"
      shift 2
      ;;
    --source-revision)
      SOURCE_REVISION="$2"
      shift 2
      ;;
    --idempotency-key)
      IDEMPOTENCY_KEY="$2"
      shift 2
      ;;
    --reason)
      REASON="$2"
      shift 2
      ;;
    --force)
      FORCE="true"
      shift
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
if [[ -z "${PUBLISH_URL}" ]]; then
  PUBLISH_URL="$(default_publish_url_for_stage "${STAGE}" || true)"
fi
PUBLISH_URL="${PUBLISH_URL%/}"
require_publish_url "${PUBLISH_URL}"

if [[ -z "${SOURCE_REVISION}" ]]; then
  SOURCE_REVISION="$(git -C "${REPO_ROOT}" rev-parse HEAD 2>/dev/null || true)"
fi
if [[ -z "${SOURCE_REVISION}" ]]; then
  fail "missing source revision"
fi

short_sha="${SOURCE_REVISION:0:12}"
if [[ -z "${IDEMPOTENCY_KEY}" ]]; then
  if [[ -n "${GITHUB_RUN_ID:-}" ]]; then
    IDEMPOTENCY_KEY="github-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT:-1}-${short_sha}"
  else
    IDEMPOTENCY_KEY="manual-${short_sha}"
  fi
fi

PAYLOAD="$(SOURCE_REVISION="${SOURCE_REVISION}" IDEMPOTENCY_KEY="${IDEMPOTENCY_KEY}" REASON="${REASON}" FORCE="${FORCE}" python3 - <<'PY'
import json
import os

payload = {
    'source_revision': os.environ['SOURCE_REVISION'],
    'idempotency_key': os.environ['IDEMPOTENCY_KEY'],
    'reason': os.environ['REASON'],
    'force': os.environ['FORCE'].strip().lower() == 'true',
}
print(json.dumps(payload, separators=(',', ':')))
PY
)"

if [[ "${PUBLISH_DRY_RUN,,}" == "true" ]]; then
  echo "trigger-theorycloud-publish: DRY RUN"
  echo "stage=${STAGE}"
  echo "url=${PUBLISH_URL}"
  echo "payload=${PAYLOAD}"
  echo "command=awscurl --service execute-api --region ${AWS_REGION} --fail-with-body -X POST -H content-type:application/json --data ${PAYLOAD} -o <response_file> ${PUBLISH_URL}"
  echo "trigger-theorycloud-publish: PASS (dry-run; url=${PUBLISH_URL})"
  exit 0
fi

command -v awscurl >/dev/null 2>&1 || fail "awscurl is required for publish invocation"

response_file="$(mktemp)"
if ! awscurl --service execute-api --region "${AWS_REGION}" --fail-with-body -X POST -H 'content-type: application/json' --data "${PAYLOAD}" -o "${response_file}" "${PUBLISH_URL}" >/dev/null; then
  status=$?
  body="$(cat "${response_file}" 2>/dev/null || true)"
  rm -f "${response_file}"
  if [[ -n "${body}" ]]; then
    fail "publish invocation failed for ${PUBLISH_URL} (exit ${status}): ${body}"
  fi
  fail "publish invocation failed for ${PUBLISH_URL} (exit ${status})"
fi

body="$(cat "${response_file}")"
rm -f "${response_file}"
echo "trigger-theorycloud-publish: PASS (url=${PUBLISH_URL})"
printf '%s\n' "${body}"
