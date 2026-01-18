#!/usr/bin/env bash
set -euo pipefail

out_file="${1:-examples/cdk-multilang/cdk.outputs.json}"

command -v jq >/dev/null 2>&1 || {
  echo "smoke: FAIL (jq not found)"
  exit 1
}
command -v curl >/dev/null 2>&1 || {
  echo "smoke: FAIL (curl not found)"
  exit 1
}
command -v aws >/dev/null 2>&1 || {
  echo "smoke: FAIL (aws CLI not found)"
  exit 1
}

if [[ ! -f "${out_file}" ]]; then
  echo "smoke: FAIL (missing outputs file: ${out_file})"
  echo "Run: AWS_PROFILE=... npm --prefix examples/cdk-multilang run deploy -- --outputs-file cdk.outputs.json"
  exit 1
fi

profile="${AWS_PROFILE:-}"
if [[ -z "${profile}" ]]; then
  echo "smoke: FAIL (AWS_PROFILE is required for the raw DynamoDB check)"
  exit 1
fi

go_url="$(jq -r '.TheorydbMultilangDemoStack.GoFunctionUrl' "${out_file}")"
node_url="$(jq -r '.TheorydbMultilangDemoStack.NodeFunctionUrl' "${out_file}")"
py_url="$(jq -r '.TheorydbMultilangDemoStack.PythonFunctionUrl' "${out_file}")"
table="$(jq -r '.TheorydbMultilangDemoStack.TableName' "${out_file}")"

if [[ -z "${go_url}" || "${go_url}" == "null" ]]; then
  echo "smoke: FAIL (missing GoFunctionUrl in ${out_file})"
  exit 1
fi
if [[ -z "${node_url}" || "${node_url}" == "null" ]]; then
  echo "smoke: FAIL (missing NodeFunctionUrl in ${out_file})"
  exit 1
fi
if [[ -z "${py_url}" || "${py_url}" == "null" ]]; then
  echo "smoke: FAIL (missing PythonFunctionUrl in ${out_file})"
  exit 1
fi
if [[ -z "${table}" || "${table}" == "null" ]]; then
  echo "smoke: FAIL (missing TableName in ${out_file})"
  exit 1
fi

pk_enc="enc-demo"
sk_enc="item-1"

echo "==> encryption: PUT via Go (/enc)"
curl -sS -X PUT "${go_url}enc" \
  -H 'content-type: application/json' \
  -d "{\"pk\":\"${pk_enc}\",\"sk\":\"${sk_enc}\",\"value\":\"v1\",\"secret\":\"shh\"}" \
  | jq -e '.ok == true and .item.secret == "shh"' >/dev/null

echo "==> encryption: GET via Node"
curl -sS "${node_url}?pk=${pk_enc}&sk=${sk_enc}" \
  | jq -e '.ok == true and .item.secret == "shh" and .item.lang == "go"' >/dev/null

echo "==> encryption: GET via Python"
curl -sS "${py_url}?pk=${pk_enc}&sk=${sk_enc}" \
  | jq -e '.ok == true and .item.secret == "shh" and .item.lang == "go"' >/dev/null

echo "==> encryption: raw DynamoDB check (secret is envelope map)"
AWS_PROFILE="${profile}" aws dynamodb get-item \
  --table-name "${table}" \
  --key "{\"PK\":{\"S\":\"${pk_enc}\"},\"SK\":{\"S\":\"${sk_enc}\"}}" \
  --consistent-read \
  --output json \
  | jq -e '.Item.secret.M.v.N == "1" and (.Item.secret.M.edk.B|length) > 0 and (.Item.secret.M.nonce.B|length) > 0 and (.Item.secret.M.ct.B|length) > 0' >/dev/null

echo "==> batch: POST via Node (/batch)"
curl -sS -X POST "${node_url}batch" \
  -H 'content-type: application/json' \
  -d '{"pk":"batch-demo","skPrefix":"B-","count":3,"value":"vb","secret":"batch-secret"}' \
  | jq -e '.ok == true and .count == 3 and (.items|length) == 3' >/dev/null

echo "==> batch: GET via Go"
curl -sS "${go_url}?pk=batch-demo&sk=B-1" \
  | jq -e '.ok == true and .item.secret == "batch-secret" and .item.lang == "ts"' >/dev/null

echo "==> tx: POST via Python (/tx)"
curl -sS -X POST "${py_url}tx" \
  -H 'content-type: application/json' \
  -d '{"pk":"tx-demo","skPrefix":"T-","value":"vt","secret":"tx-secret"}' \
  | jq -e '.ok == true and .count == 2 and (.items|length) == 2' >/dev/null

echo "==> tx: GET via Node"
curl -sS "${node_url}?pk=tx-demo&sk=T-1" \
  | jq -e '.ok == true and .item.secret == "tx-secret" and .item.lang == "py"' >/dev/null

echo "smoke: PASS"

