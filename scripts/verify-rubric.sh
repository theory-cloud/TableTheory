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

# R-G1: the report the verifier just wrote must be a well-formed
# gov_rubric_report.v1 document. The self-test proves the validator is not
# vacuous (it rejects known mutations) before the real report is validated.
python3 scripts/verify-gov-rubric-report.py --self-test
python3 scripts/verify-gov-rubric-report.py gov-infra/evidence/gov-rubric-report.json

bash ./scripts/verify-theorycloud-tabletheory-subtree.sh

# TTSEC-M0-T2..T6 focused policy tests (release/publishing trust boundaries).
# These exercise hostile fixtures against the real guards, so they run in the
# full rubric on staging PRs rather than only as existence checks.
bash ./scripts/test-release-pr-postcondition-policy.sh
bash ./scripts/test-theorycloud-publish-policy.sh
bash ./scripts/test-consumer-dependency-policy.sh

# TTSEC2-M3-T2 Pages publication trust boundary: no workflow may deploy GitHub
# Pages from a `staging` or `pull_request` trigger, and build-only runs must
# never share the publication concurrency queue. Hostile fixtures prove the
# checker is not vacuous before the real workflows are checked.
bash ./scripts/test-pages-publication-policy.sh

# Preserve legacy success line for scripts that grep for it.
echo "rubric: PASS"
