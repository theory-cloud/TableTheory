#!/usr/bin/env bash
# Smoke test for `tabletheory init`: generate the Go quickstart scaffold and
# prove it reaches a successful CRUD write against a throwaway DynamoDB Local.
#
# First prove the source-build default resolves as a published release without
# a local replacement. Then generate an explicit current-major scaffold and
# point that one at the working tree so its CRUD smoke validates current source.
# Release-built CLIs override the default with their own tag through ldflags.
# DynamoDB Local runs on an isolated port so no AWS account is required.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

PORT="${INIT_SMOKE_PORT:-8055}"
ENDPOINT="http://localhost:${PORT}"
GOTOOLCHAIN_PIN="$(awk '/^toolchain /{print $2; exit}' go.mod)"
export GOTOOLCHAIN="${GOTOOLCHAIN_PIN:-local}"

WORKDIR="$(mktemp -d)"
DEFAULT_SCAFFOLD="${WORKDIR}/default-app"
CURRENT_SCAFFOLD="${WORKDIR}/current-app"
CID=""

cleanup() {
  if [[ -n "$CID" ]]; then
    docker stop "$CID" >/dev/null 2>&1 || true
  fi
  rm -rf "$WORKDIR"
}
trap cleanup EXIT

echo "==> building tabletheory CLI"
go build -o "${WORKDIR}/tabletheory" ./cmd/tabletheory

echo "==> scaffolding and resolving the published default"
"${WORKDIR}/tabletheory" init \
  --lang go \
  --dir "$DEFAULT_SCAFFOLD" \
  --module example.com/tabletheory-init-default
(
  cd "$DEFAULT_SCAFFOLD"
  go mod download
)

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

echo "==> scaffolding Go quickstart"
"${WORKDIR}/tabletheory" init \
  --lang go \
  --dir "$CURRENT_SCAFFOLD" \
  --module example.com/tabletheory-init-smoke \
  --runtime-version 3.0.0

echo "==> resolving against the working tree"
(
  cd "$CURRENT_SCAFFOLD"
  go mod edit -replace "github.com/theory-cloud/tabletheory/v3=${REPO_ROOT}"
  go mod tidy
)

echo "==> running scaffold CRUD program"
OUTPUT="$(cd "$CURRENT_SCAFFOLD" && \
  AWS_ACCESS_KEY_ID=dummy AWS_SECRET_ACCESS_KEY=dummy AWS_REGION=us-east-1 \
  DYNAMODB_ENDPOINT="$ENDPOINT" go run .)"
echo "$OUTPUT"

if ! grep -q "OK: TableTheory CRUD against DynamoDB Local succeeded" <<<"$OUTPUT"; then
  echo "FAIL: scaffold did not complete a CRUD write" >&2
  exit 1
fi

echo "PASS: tabletheory init Go scaffold reached a CRUD write against DynamoDB Local"
