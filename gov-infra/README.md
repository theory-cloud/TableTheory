# gov-infra (`gov-infra/`)

This directory contains **repo-local governance artifacts** managed by gov-infra. It is designed to make
quality/security/compliance work explicit, versioned, and resistant to “green by exclusion”.

## Quick Start

From the repository root:

1) Run the deterministic rubric verifier:
   - `bash gov-infra/verifiers/gov-verify-rubric.sh`
2) Read the machine report:
   - `gov-infra/evidence/gov-rubric-report.json`
3) Inspect evidence logs for each rubric ID:
   - `gov-infra/evidence/*-output.log`

Notes:
- Verifier scripts are intentionally safe to commit **without** execute permissions; always run via `bash …`.
- Missing or unimplemented checks should be recorded as `BLOCKED`, not “fixed” by weakening gates.
- Supply-chain checks may use the allowlist with justification: `gov-infra/planning/theorydb-supply-chain-allowlist.txt`.
- Upstream-bundled findings that must stay visible in green SEC-2 evidence are tracked separately in
  `gov-infra/planning/theorydb-visible-npm-audit-findings.json`; do not copy those findings into the suppression
  allowlist.

## What’s In Here

- `gov-infra/pack.json`: provenance + the source of truth for the governed artifact set (additive-only).
- `gov-infra/planning/`: threat model, controls matrix, rubric, roadmap, evidence plan, drift recovery.
- `gov-infra/verifiers/`: deterministic verifier entrypoints (run via `bash`, not `./`).
- `gov-infra/evidence/`: reports + logs written by verifiers (deterministic paths).
- `gov-infra/prompts/`: optional JSON prompt/spec blobs for user agents (generated server-side).

## Working Rules (Humans + Agents)

- Keep all governance outputs under `gov-infra/`.
- Treat the rubric/roadmap as living documents: they are not static; keep them versioned in git and evolve them intentionally.
- Don’t relax gates by excluding directories, lowering thresholds, or disabling checks.
- If a verifier depends on a missing tool/version pin, **fail closed** (`BLOCKED`) until pinned.
- Don’t commit secrets into `gov-infra/`.
- If you suppress a supply-chain finding, add the exact ID to `gov-infra/planning/theorydb-supply-chain-allowlist.txt` with a justification comment.
- If a supply-chain finding is accepted only because it is an unfixed upstream bundle, add it to
  `gov-infra/planning/theorydb-visible-npm-audit-findings.json` instead. The SEC-2 log must still print the exact finding
  and must not call it allowlisted.

## Next Steps

- Open `gov-infra/planning/` and follow the next step in the roadmap.
- Run `bash gov-infra/verifiers/gov-verify-rubric.sh` for CI-friendly deterministic validation.
- Signing is retired for this MCP-managed governance surface; do not create or refresh signature bundles.
