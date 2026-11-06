#!/usr/bin/env bash
# Local, no-AWS replacement for scripts/smoke.sh.
#
# scripts/smoke.sh targets a deployed stack: it needs a cdk.outputs.json, an
# AWS profile, IAM-signed Function URLs and a live DynamoDB table. This script
# drives the same journeys (encrypted write, cross-language reads, the raw
# envelope, batch and transaction) against the same handler code started in the
# Lambda runtime interface by the examples harness. The tables are real
# DynamoDB Local tables. The harness row also exercises the archive event source
# with a real DynamoDB stream REMOVE record. No SigV4 is needed because the
# local RIE endpoints are plain loopback HTTP; Function URL IAM policy
# authentication is the only remaining named gap (docs/125: IAM policy
# evaluation).
#
# Required environment (set by the harness):
#   CDK_GO_URL, CDK_NODE_URL, CDK_PY_URL  RIE invocation URLs for each handler
#   TABLE_NAME                            shared demo table
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"

: "${CDK_GO_URL:?CDK_GO_URL is required}"
: "${CDK_NODE_URL:?CDK_NODE_URL is required}"
: "${CDK_PY_URL:?CDK_PY_URL is required}"
: "${TABLE_NAME:?TABLE_NAME is required}"

command -v jq >/dev/null 2>&1 || {
  echo "smoke-local: FAIL (jq not found)"
  exit 1
}
command -v curl >/dev/null 2>&1 || {
  echo "smoke-local: FAIL (curl not found)"
  exit 1
}

# invoke URL METHOD PATH [BODY] [QUERY]
# Builds the Lambda Function URL event the deployed handlers receive and posts
# it to the RIE invocation endpoint.
invoke() {
  local url="$1" method="$2" path="$3" body="${4:-}" query="${5:-}"
  if [[ -z "$query" ]]; then
    query='{}'
  fi
  local event
  event="$(jq -n \
    --arg method "$method" \
    --arg path "$path" \
    --arg body "$body" \
    --argjson query "$query" \
    '{rawPath: $path, requestContext: {http: {method: $method}}, headers: {"content-type": "application/json"}, body: $body, queryStringParameters: $query}')"
  curl --silent --show-error --fail-with-body -X POST \
    -H 'content-type: application/json' \
    --data-binary "$event" \
    "$url"
}

# body_of RESPONSE prints the JSON body a handler returned.
body_of() {
  jq -r '.body'
}

echo "==> encryption: PUT via Go (/enc)"
response="$(invoke "$CDK_GO_URL" PUT /enc \
  '{"pk":"enc-demo","sk":"item-1","value":"v1","secret":"shh"}')"
jq -e '.statusCode == 200' <<<"$response" >/dev/null
body_of <<<"$response" | jq -e '.ok == true and .item.secret == "shh"' >/dev/null

echo "==> encryption: GET via Node"
response="$(invoke "$CDK_NODE_URL" GET / '{}' '{"pk":"enc-demo","sk":"item-1"}')"
jq -e '.statusCode == 200' <<<"$response" >/dev/null
body_of <<<"$response" | jq -e '.ok == true and .item.secret == "shh" and .item.lang == "go"' >/dev/null

echo "==> encryption: GET via Python"
response="$(invoke "$CDK_PY_URL" GET / '{}' '{"pk":"enc-demo","sk":"item-1"}')"
jq -e '.statusCode == 200' <<<"$response" >/dev/null
body_of <<<"$response" | jq -e '.ok == true and .item.secret == "shh" and .item.lang == "go"' >/dev/null

echo "==> encryption: raw DynamoDB check (secret is envelope map)"
envelope="$(cd "$REPO_ROOT" && go run ./scripts/local/cmd/examples-local-cdk-fixtures \
  -mode verify-envelope -pk enc-demo -sk item-1 -plaintext shh)"
jq -e '.found == true and (.envelope_version == 1 or .envelope_version == 2) and .edk_bytes > 0 and .nonce_bytes > 0 and .ct_bytes > 0' \
  <<<"$envelope" >/dev/null

echo "==> batch: POST via Node (/batch)"
response="$(invoke "$CDK_NODE_URL" POST /batch \
  '{"pk":"batch-demo","skPrefix":"B-","count":3,"value":"vb","secret":"batch-secret"}')"
jq -e '.statusCode == 200' <<<"$response" >/dev/null
body_of <<<"$response" | jq -e '.ok == true and .count == 3 and (.items|length) == 3' >/dev/null

echo "==> batch: GET via Go"
response="$(invoke "$CDK_GO_URL" GET / '{}' '{"pk":"batch-demo","sk":"B-1"}')"
jq -e '.statusCode == 200' <<<"$response" >/dev/null
body_of <<<"$response" | jq -e '.ok == true and .item.secret == "batch-secret" and .item.lang == "ts"' >/dev/null

echo "==> tx: POST via Python (/tx)"
response="$(invoke "$CDK_PY_URL" POST /tx \
  '{"pk":"tx-demo","skPrefix":"T-","value":"vt","secret":"tx-secret"}')"
jq -e '.statusCode == 200' <<<"$response" >/dev/null
body_of <<<"$response" | jq -e '.ok == true and .count == 2 and (.items|length) == 2' >/dev/null

echo "==> tx: GET via Node"
response="$(invoke "$CDK_NODE_URL" GET / '{}' '{"pk":"tx-demo","sk":"T-1"}')"
jq -e '.statusCode == 200' <<<"$response" >/dev/null
body_of <<<"$response" | jq -e '.ok == true and .item.secret == "tx-secret" and .item.lang == "py"' >/dev/null

echo "smoke-local: PASS"
