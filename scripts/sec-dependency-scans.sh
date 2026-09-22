#!/usr/bin/env bash
set -euo pipefail

bash scripts/sec-govulncheck.sh
bash scripts/test-npm-audit-policy.sh
bash scripts/sec-npm-audit.sh
bash scripts/test-npm-engines-floor-policy.sh
bash scripts/verify-npm-engines-floor.sh
bash scripts/test-lambda-runtime-deprecations-policy.sh
bash scripts/verify-lambda-runtime-deprecations.sh
bash scripts/sec-pip-audit.sh

echo "dependency-scans: PASS"
