#!/usr/bin/env bash
# Local, no-AWS variant of the cdk-multilang demo.
#
# Proves the cross-language "no drift" property against a throwaway DynamoDB
# Local and the local KMS: the Go runtime writes one shared item with an
# encrypted secret, then the TypeScript and Python runtimes read it back,
# decrypt the secret through KMS, and assert the same shape and plaintext. It
# also drift-checks the committed generated CDK construct against dms/demo.yml.
#
# KMS_KEY_ARN must point at a key in the local KMS (the examples harness
# provisions one with scripts/local/cmd/examples-local-kms-key). The script
# never contacts AWS.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
LOCAL_DIR="${REPO_ROOT}/examples/cdk-multilang/local"
PORT="${CDK_LOCAL_PORT:-}"

if [[ -z "${KMS_KEY_ARN:-}" ]]; then
  echo "KMS_KEY_ARN is required for the encrypted local variant" >&2
  exit 1
fi

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
  npm --prefix "${REPO_ROOT}/ts" ci --ignore-scripts
fi

echo "==> starting throwaway DynamoDB Local"
# Publish on loopback only: DynamoDB Local enforces no authentication, so the
# throwaway instance must not be reachable from other network interfaces. Unless
# the caller pinned a port, Docker chooses an ephemeral one so two local runs
# cannot collide.
DDB_IMAGE="amazon/dynamodb-local@sha256:7ef4a2c45b58c2901e70a4f28e0953a422c2c631baaaf5e2c15e0805740c7752"
if [[ -n "$PORT" ]]; then
  CID="$(docker run -d --rm -p "127.0.0.1:${PORT}:8000" "$DDB_IMAGE" \
    -jar DynamoDBLocal.jar -sharedDb -inMemory)"
else
  CID="$(docker run -d --rm -p "127.0.0.1::8000" "$DDB_IMAGE" \
    -jar DynamoDBLocal.jar -sharedDb -inMemory)"
fi
if [[ -z "$PORT" ]]; then
  PORT="$(docker port "$CID" 8000 | head -n1 | sed 's/.*://')"
fi
ENDPOINT="http://127.0.0.1:${PORT}"
export DYNAMODB_ENDPOINT="$ENDPOINT"

echo "==> waiting for DynamoDB Local on ${ENDPOINT}"
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

echo "==> go: create shared table and write item (encrypted secret)"
(cd "$REPO_ROOT" && go run ./examples/cdk-multilang/local/go)

echo "==> node: read the same item via the TypeScript runtime"
"${REPO_ROOT}/ts/node_modules/.bin/tsx" "${LOCAL_DIR}/read.mts"

echo "==> python: read the same item via the Python runtime"
uv --directory "${REPO_ROOT}/py" run python "${LOCAL_DIR}/read.py"

echo "LOCAL CROSS-LANGUAGE NO-DRIFT: PASS"
