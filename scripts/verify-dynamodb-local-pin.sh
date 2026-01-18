#!/usr/bin/env bash
set -euo pipefail

# Ensures DynamoDB Local is pinned (no :latest) across docker-compose and local dev fallbacks.

failures=0

if [[ -f docker-compose.yml ]]; then
  if grep -Eq 'image:\s*amazon/dynamodb-local:latest\b' docker-compose.yml; then
    echo "dynamodb-local: docker-compose.yml uses :latest (pin a version)"
    failures=$((failures + 1))
  fi
  if grep -Eq 'image:\s*amazon/dynamodb-local([[:space:]]|$)' docker-compose.yml; then
    echo "dynamodb-local: docker-compose.yml uses untagged image (implicit :latest; pin a version)"
    failures=$((failures + 1))
  fi
fi

if [[ -f Makefile ]]; then
  if grep -Eq '^DYNAMODB_LOCAL_IMAGE\s*\?=\s*amazon/dynamodb-local:latest\b' Makefile; then
    echo "dynamodb-local: Makefile DYNAMODB_LOCAL_IMAGE uses :latest (pin a version)"
    failures=$((failures + 1))
  fi
  if grep -Eq '^DYNAMODB_LOCAL_IMAGE\s*\?=\s*amazon/dynamodb-local([[:space:]]|$)' Makefile; then
    echo "dynamodb-local: Makefile DYNAMODB_LOCAL_IMAGE is untagged (implicit :latest; pin a version)"
    failures=$((failures + 1))
  fi
fi

# Scan repo for other docker-compose usage, but exclude documentation markdown to avoid false positives.
if rg -n --no-heading \
  --glob '!scripts/verify-dynamodb-local-pin.sh' \
  --glob '!**/*.md' \
  'amazon/dynamodb-local:latest\\b' . >/dev/null 2>&1; then
  echo "dynamodb-local: found amazon/dynamodb-local:latest in repo (pin a version)"
  rg -n \
    --glob '!scripts/verify-dynamodb-local-pin.sh' \
    --glob '!**/*.md' \
    'amazon/dynamodb-local:latest\\b' .
  failures=$((failures + 1))
fi

if rg -n --no-heading \
  --glob '!scripts/verify-dynamodb-local-pin.sh' \
  --glob '!**/*.md' \
  'amazon/dynamodb-local([[:space:]]|$)' . >/dev/null 2>&1; then
  echo "dynamodb-local: found untagged amazon/dynamodb-local in repo (implicit :latest; pin a version)"
  rg -n \
    --glob '!scripts/verify-dynamodb-local-pin.sh' \
    --glob '!**/*.md' \
    'amazon/dynamodb-local([[:space:]]|$)' .
  failures=$((failures + 1))
fi

if [[ "${failures}" -ne 0 ]]; then
  echo "dynamodb-local: FAIL (${failures} issue(s))"
  exit 1
fi

echo "dynamodb-local: pinned"
