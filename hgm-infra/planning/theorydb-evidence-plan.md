# theorydb Evidence Plan (Rubric v0.2)

Defines where evidence for rubric items is produced and how to regenerate it. Evidence should be reproducible from a commit SHA (no hand-assembled screenshots unless unavoidable).

## Evidence sources
### CI artifacts (preferred)
- Coverage: `bash scripts/verify-coverage.sh` → `coverage_lib.out` (plus `ts/coverage` and `py/coverage.xml` if applicable)
- Lint: `bash scripts/verify-lint.sh` output (pinned version)
- Security: `bash scripts/sec-gosec.sh`, `bash scripts/sec-dependency-scans.sh` (logs; optional SARIF/JSON artifacts)
- Supply-chain: `go mod verify`

### Deterministic in-repo artifacts
- Controls matrix: `hgm-infra/planning/theorydb-controls-matrix.md`
- Rubric: `hgm-infra/planning/theorydb-10of10-rubric.md`
- Roadmap: `hgm-infra/planning/theorydb-10of10-roadmap.md`
- Evidence plan: `hgm-infra/planning/theorydb-evidence-plan.md`
- Supply-chain allowlist: `hgm-infra/planning/theorydb-supply-chain-allowlist.txt`
- Threat model: `hgm-infra/planning/theorydb-threat-model.md`
- Logging/ops standards: `hgm-infra/planning/theorydb-logging-ops-standards.md`
- Maintainability roadmap: `hgm-infra/planning/theorydb-maintainability-roadmap.md`
- AI drift recovery: `hgm-infra/planning/theorydb-ai-drift-recovery.md`
- Signature bundle (local certification): `hgm-infra/signatures/hgm-signature-bundle.json`

## Rubric-to-evidence map
Every rubric ID maps to exactly one verifier and one primary evidence location.

| Rubric ID | Primary evidence | Evidence path | How to refresh |
| --- | --- | --- | --- |
| QUA-1 | Unit test output | `hgm-infra/evidence/QUA-1-output.log` | `bash scripts/verify-unit-tests.sh` |
| QUA-2 | Integration test output | `hgm-infra/evidence/QUA-2-output.log` | `bash scripts/verify-integration-tests.sh` |
| QUA-3 | Coverage profile + summary | `hgm-infra/evidence/QUA-3-output.log` | `bash scripts/verify-coverage.sh` |
| QUA-4 | Validator ↔ converter parity output | `hgm-infra/evidence/QUA-4-output.log` | `bash scripts/verify-validation-parity.sh` |
| QUA-5 | Fuzz smoke output | `hgm-infra/evidence/QUA-5-output.log` | `bash scripts/fuzz-smoke.sh` |
| CON-1 | Formatter diff list | `hgm-infra/evidence/CON-1-output.log` | `bash scripts/verify-formatting.sh` |
| CON-2 | Lint output | `hgm-infra/evidence/CON-2-output.log` | `bash scripts/verify-lint.sh` |
| CON-3 | Contract verification output | `hgm-infra/evidence/CON-3-output.log` | `bash scripts/verify-public-api-contracts.sh`, `bash scripts/verify-dms-first-workflow.sh` |
| COM-1 | Builds + version alignment output | `hgm-infra/evidence/COM-1-output.log` | `bash scripts/verify-typescript-deps.sh`, `bash scripts/verify-python-deps.sh`, `bash scripts/verify-builds.sh` |
| COM-2 | Toolchain pin verification | `hgm-infra/evidence/COM-2-output.log` | `bash scripts/verify-ci-toolchain.sh` |
| COM-3 | Planning docs presence | `hgm-infra/evidence/COM-3-output.log` | `bash scripts/verify-planning-docs.sh` |
| COM-4 | Lint config validation | `hgm-infra/evidence/COM-4-output.log` | `golangci-lint config verify -c .golangci-v2.yml` |
| COM-5 | Coverage threshold check | `hgm-infra/evidence/COM-5-output.log` | `bash scripts/verify-coverage-threshold.sh` |
| COM-6 | CI rubric enforcement check | `hgm-infra/evidence/COM-6-output.log` | `bash scripts/verify-ci-rubric-enforced.sh` |
| COM-7 | DynamoDB Local pin check | `hgm-infra/evidence/COM-7-output.log` | `bash scripts/verify-dynamodb-local-pin.sh` |
| COM-8 | Branch/release supply-chain + version sync | `hgm-infra/evidence/COM-8-output.log` | `bash scripts/verify-branch-release-supply-chain.sh`, `bash scripts/verify-branch-version-sync.sh` |
| SEC-1 | SAST scan output | `hgm-infra/evidence/SEC-1-output.log` | `bash scripts/sec-gosec.sh` |
| SEC-2 | Vulnerability scan output | `hgm-infra/evidence/SEC-2-output.log` | `bash scripts/sec-dependency-scans.sh` |
| SEC-3 | Module integrity verification | `hgm-infra/evidence/SEC-3-output.log` | `go mod verify` |
| SEC-4 | No-panics scan output | `hgm-infra/evidence/SEC-4-output.log` | `bash scripts/verify-no-panics.sh` |
| SEC-5 | Safe-defaults enforcement output | `hgm-infra/evidence/SEC-5-output.log` | `bash scripts/verify-safe-defaults.sh` |
| SEC-6 | Expression hardening output | `hgm-infra/evidence/SEC-6-output.log` | `bash scripts/verify-expression-hardening.sh` |
| SEC-7 | Network hygiene output | `hgm-infra/evidence/SEC-7-output.log` | `bash scripts/verify-network-hygiene.sh` |
| SEC-8 | Encrypted tag regression output | `hgm-infra/evidence/SEC-8-output.log` | `bash scripts/verify-encrypted-tag-implemented.sh` |
| SEC-9 | Logging/ops standards output | `hgm-infra/evidence/SEC-9-output.log` | `check_logging_ops_standards` (via HGM verifier) |
| CMP-1 | Controls matrix exists | `hgm-infra/planning/theorydb-controls-matrix.md` | File existence check |
| CMP-2 | Evidence plan exists | `hgm-infra/planning/theorydb-evidence-plan.md` | File existence check |
| CMP-3 | Threat model exists | `hgm-infra/planning/theorydb-threat-model.md` | File existence check |
| MAI-1 | File budget check | `hgm-infra/evidence/MAI-1-output.log` | `bash scripts/verify-file-size.sh` |
| MAI-2 | Maintainability roadmap check | `hgm-infra/evidence/MAI-2-output.log` | `bash scripts/verify-maintainability-roadmap.sh` |
| MAI-3 | Singleton check | `hgm-infra/evidence/MAI-3-output.log` | `bash scripts/verify-query-singleton.sh` |
| DOC-1 | Threat model present | `hgm-infra/planning/theorydb-threat-model.md` | File existence check |
| DOC-2 | Evidence plan present | `hgm-infra/planning/theorydb-evidence-plan.md` | File existence check |
| DOC-3 | Rubric + roadmap present | `hgm-infra/planning/theorydb-10of10-rubric.md` | File existence check |
| DOC-4 | Doc integrity output | `hgm-infra/evidence/DOC-4-output.log` | `check_doc_integrity` (via HGM verifier) |
| DOC-5 | Threat ↔ controls parity output | `hgm-infra/evidence/DOC-5-output.log` | `check_threat_controls_parity_full` (via HGM verifier) |

## Rubric Report (Fixed Location)
The deterministic verifier (`hgm-infra/verifiers/hgm-verify-rubric.sh`) produces a machine-readable report at:
- `hgm-infra/evidence/hgm-rubric-report.json`

## Notes
- All evidence paths must live under `hgm-infra/`.
- Treat evidence refresh as part of `hgm validate`; CI should archive artifacts.
