#!/usr/bin/env bash
set -euo pipefail

# Legacy rubric entrypoint (kept for backwards compatibility).
#
# Canonical runner is the deterministic gov-infra verifier:
#   `bash gov-infra/verifiers/gov-verify-rubric.sh`
#
# This wrapper preserves the legacy `make rubric` surface while ensuring all
# gates produce evidence under `gov-infra/evidence/`.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

GO_TOOL_BIN="$(command -v go 2>/dev/null || true)"
if [[ -n "${GO_TOOL_BIN}" ]]; then
  GO_TOOL_DIR="$(dirname "${GO_TOOL_BIN}")"
  export PATH="${GO_TOOL_DIR}:${PATH}"
fi

GO_BIN_DIR="$(go env GOBIN)"
if [[ -z "${GO_BIN_DIR}" ]]; then
  GO_BIN_DIR="$(go env GOPATH)/bin"
fi
export PATH="${GO_BIN_DIR}:${PATH}"

bash gov-infra/verifiers/gov-verify-rubric.sh
bash ./scripts/verify-theorycloud-tabletheory-subtree.sh

# Preserve legacy success line for scripts that grep for it.
echo "rubric: PASS"
