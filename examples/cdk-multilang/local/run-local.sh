#!/usr/bin/env bash
# Local, no-AWS variant of the cdk-multilang demo.
#
# Proves the cross-language "no drift" property against a throwaway DynamoDB
# Local: the Go runtime writes one shared item, then the TypeScript and Python
# runtimes read it back and assert the same shape. It also drift-checks the
# committed generated CDK construct against dms/demo.yml.
#
# Encryption is intentionally out of scope here: encrypted fields fail closed
# without KMS, which is not available in a no-AWS run. Use the deployed AWS
# variant (scripts/smoke.sh) to exercise encryption end to end.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
LOCAL_DIR="${REPO_ROOT}/examples/cdk-multilang/local"
PORT="${CDK_LOCAL_PORT:-8020}"
ENDPOINT="http://localhost:${PORT}"

export DYNAMODB_ENDPOINT="$ENDPOINT"
export AWS_REGION="${AWS_REGION:-us-east-1}"
export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-dummy}"
export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-dummy}"
export DEMO_TABLE_NAME="${DEMO_TABLE_NAME:-demo_multilang_local}"
export GOTOOLCHAIN="$(awk '/^toolchain /{print $2; exit}' "$REPO_ROOT/go.mod")"

CID=""
cleanup() {
  if [[ -n "$CID" ]]; then
    docker stop "$CID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

echo "==> drift-checking generated CDK construct against dms/demo.yml"
generated="$(cd "$REPO_ROOT" && go run ./cmd/tabletheory gen --cdk examples/cdk-multilang/dms/demo.yml)"
if ! diff <(printf '%s\n' "$generated") "${LOCAL_DIR}/generated-table.ts" >/dev/null; then
  echo "FAIL: local/generated-table.ts is stale; regenerate with:" >&2
  echo "  tabletheory gen --cdk examples/cdk-multilang/dms/demo.yml > examples/cdk-multilang/local/generated-table.ts" >&2
  exit 1
fi

if [[ ! -x "${REPO_ROOT}/ts/node_modules/.bin/tsx" ]]; then
  echo "==> installing TypeScript runtime dev deps"
  npm --prefix "${REPO_ROOT}/ts" ci
fi

echo "==> starting throwaway DynamoDB Local on port ${PORT}"
CID="$(docker run -d --rm -p "${PORT}:8000" amazon/dynamodb-local:3.1.0 \
  -jar DynamoDBLocal.jar -sharedDb -inMemory)"

echo "==> waiting for DynamoDB Local"
ready=""
for _ in $(seq 1 30); do
  if curl -sS --max-time 2 -o /dev/null "$ENDPOINT"; then
    ready="yes"
    break
  fi
  sleep 1
done
if [[ -z "$ready" ]]; then
  echo "DynamoDB Local did not become ready on ${ENDPOINT}" >&2
  exit 1
fi

echo "==> go: create shared table and write item"
(cd "$REPO_ROOT" && go run ./examples/cdk-multilang/local/go)

echo "==> node: read the same item via the TypeScript runtime"
"${REPO_ROOT}/ts/node_modules/.bin/tsx" "${LOCAL_DIR}/read.mts"

echo "==> python: read the same item via the Python runtime"
uv --directory "${REPO_ROOT}/py" run python "${LOCAL_DIR}/read.py"

echo "LOCAL CROSS-LANGUAGE NO-DRIFT: PASS"
