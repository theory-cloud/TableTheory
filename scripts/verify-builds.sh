#!/usr/bin/env bash
set -euo pipefail

bash scripts/verify-go-modules.sh
bash scripts/verify-go-semantic-import-version.sh
bash scripts/verify-typescript-build.sh
bash scripts/verify-python-build.sh
bash scripts/verify-branch-version-sync.sh --alignment
bash scripts/verify-cdk-synth.sh

echo "builds: PASS"
