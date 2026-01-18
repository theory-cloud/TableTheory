#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f "ts/package.json" ]]; then
  echo "typescript-integration: SKIP (ts/package.json not found)"
  exit 0
fi

if [[ ! -d "ts/node_modules" ]]; then
  bash scripts/verify-typescript-deps.sh
fi

export AWS_REGION="${AWS_REGION:-us-east-1}"
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"
export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-dummy}"
export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-dummy}"
export DYNAMODB_ENDPOINT="${DYNAMODB_ENDPOINT:-http://localhost:8000}"

npm --prefix ts run test:integration

echo "typescript-integration: PASS"

