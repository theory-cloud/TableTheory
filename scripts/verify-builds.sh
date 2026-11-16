#!/usr/bin/env bash
set -euo pipefail

# Composition, not a second execution: when the rubric gate composes this
# gate's reviewed record, the record is verified against these exact inputs and
# this script exits without running its commands again. A record that does not
# apply (stale, missing, failed, or another profile) makes this a no-op and the
# real command below runs.
if bash scripts/evidence/reuse-if-recorded.sh builds; then
  echo "builds: PASS (reused the reviewed evidence record)"
  exit 0
fi

bash scripts/verify-go-modules.sh
bash scripts/verify-go-semantic-import-version.sh
bash scripts/verify-typescript-build.sh
bash scripts/verify-python-build.sh
bash scripts/verify-branch-version-sync.sh --alignment
bash scripts/verify-cdk-synth.sh

echo "builds: PASS"
