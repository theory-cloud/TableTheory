#!/usr/bin/env bash
set -euo pipefail

bash scripts/verify-go-modules.sh
bash scripts/verify-typescript-build.sh
bash scripts/verify-python-build.sh
bash scripts/verify-version-alignment.sh
bash scripts/verify-cdk-synth.sh

echo "builds: PASS"
