#!/usr/bin/env bash
set -euo pipefail

bash scripts/sec-govulncheck.sh
bash scripts/sec-npm-audit.sh
bash scripts/sec-pip-audit.sh

echo "dependency-scans: PASS"
