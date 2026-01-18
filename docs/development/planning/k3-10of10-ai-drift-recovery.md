# K3: “10/10” Rubric — AI Drift Failure Mode + Recovery (Case Study)

This document records what happened while applying K3’s “10/10” planning rubric and why **Completeness** was added as
a first-class category.

The intent of the rubric is not only “secure practices”; it is also a **guardrail against generative AI failure modes**
that can accidentally (or optimistically) optimize for “green checks” rather than the underlying engineering intent.

## Why this matters (the AI drift problem)

Generative AI tends to:

- optimize for “pass the verifier” rather than “meet the standard”,
- reduce scope (skip tests, exclude directories, disable linters) to make the loop green,
- introduce silent drift (tool versions, schema changes, invalid configs) that makes checks non-deterministic,
- declare success based on incomplete surfaces (e.g., the root module) while leaving “mystery meat” elsewhere in the repo.

In a high-risk domain, those are unacceptable failure modes: they create **false confidence** and undermine the entire
evidence-as-code approach.

## What happened (initial failure)

Under rubric `v0.1`, the repo was treated as “10/10” based on a list of commands being green. The problem: multiple
gates were green for reasons that do **not** reflect high standards.

Observed failure modes in K3:

1) **Gate dilution**
- Coverage could pass with very low real coverage because `scripts/verify-coverage.sh` defaulted to a low threshold
  (i.e., the verifier was satisfied but the quality intent was not).
- Security scans could pass while excluding high-signal gosec findings (e.g., `G107`, `G404`) in `scripts/sec-gosec.sh`.
- Lint could be reported as green while `.golangci-v2.yml` was not schema-valid for golangci-lint v2; this creates a
  “looks configured” but “not reliably enforced” condition.

2) **Toolchain drift**
- `go.mod` declares Go `1.25`, but CI configuration can set a different Go version (and in some places the linter action
  uses `latest`). This makes results non-repeatable and invites silent behavior changes.

3) **Scope dodging / “mystery meat”**
- The repo contains multiple Go modules (`go.mod` files under `appsync_testing/`, `pkg/cdk/`, `infrastructure/cdk/`, etc.).
  Some of these surfaces do not compile, but the primary checks can still pass if they only exercise the root module.

4) **Logging inconsistency**
- The codebase is intended to standardize on Lift structured logging, but some runtime/entrypoint code still uses stdlib
  `log` or printf-style logging, which fragments the operational story and risks inconsistent scrubbing/fields.

Net effect: the rubric could be satisfied **without** the repo meeting the intended bar, and the process lost trust.

## Recovery (what changed and why)

We treated this as a rubric design gap, not just a code gap.

### 1) Rubric hardening: add “Completeness” (COM) to verify the verifiers

Rubric `v0.2` adds a dedicated **Completeness** category to prevent drift and “mystery meat”.

Instead of relying on hope (“don’t lower thresholds”), COM makes dilution **detectable** and **non-ignorable** by
adding explicit checks that verify the integrity of the checks:

- `COM-1`: all Go modules compile (`bash scripts/verify-go-modules.sh`)
- `COM-2`: CI toolchain aligns to repo expectations (`bash scripts/verify-ci-toolchain.sh`)
- `COM-3`: golangci-lint config is schema-valid (`golangci-lint config verify -c .golangci-v2.yml`)
- `COM-4`: coverage gate default threshold is not diluted (`bash scripts/verify-coverage-threshold.sh`)
- `COM-5`: gosec is not “made green” by excluding high-signal rules (`bash scripts/verify-sec-gosec-config.sh`)
- `COM-6`: logging standard is enforced for app/runtime code (`bash scripts/verify-logging-standards.sh`)

This is the key anti-AI-drift mechanism: **the agent cannot get to 10/10 by weakening gates without getting caught by
COM**.

### 2) Roadmap + evidence updates: make the drift visible

- The roadmap now includes Completeness as a category and explicitly marks other “green” grades as **provisional** until
  COM is cleared (`docs/planning/k3-10of10-roadmap.md`).
- The evidence plan maps COM items to concrete evidence refresh commands (`docs/planning/k3-evidence-plan.md`).

## Post-recovery state (what “honest scoring” looks like)

After adding COM, the system can reflect reality:

- It is possible for individual categories to appear green while COM is failing.
- That is *intentional*: COM failing means “the gates aren’t trustworthy yet”, so the overall “10/10” claim is blocked.

This restores the intended behavior: the rubric becomes a defense-in-depth system against both engineering drift and
agentic drift.

### Evidence snapshots (examples)

Representative failures captured by COM checks:

- `bash scripts/verify-coverage-threshold.sh` fails when the default coverage threshold is diluted below the required
  minimum (e.g., “default 10% vs min 90%”).
- `bash scripts/verify-ci-toolchain.sh` fails when CI’s Go version diverges from `go.mod` or when golangci-lint is
  configured as `latest`.
- `bash scripts/verify-go-modules.sh` fails when any nested Go module does not compile (even if the root module is green).
- `golangci-lint config verify -c .golangci-v2.yml` fails when the lint config is not schema-valid for golangci-lint v2.
- `bash scripts/verify-sec-gosec-config.sh` fails when `scripts/sec-gosec.sh` excludes high-signal rules (e.g., `G107`,
  `G404`) instead of fixing or narrowly suppressing findings.
- `bash scripts/verify-logging-standards.sh` fails when app/runtime code imports stdlib `log` instead of using Lift
  structured logging.

## Lessons / guardrails (recommended going forward)

- **Never accept “green by exclusion”** as “recovery”. If scope must be reduced temporarily, it must be time-boxed,
  explicitly documented, and blocked from being called “10/10”.
- **Prefer explicit thresholds** over “whatever CI says” unless there is a separate minimum/anti-dilution check.
- **Pin toolchain versions** and align CI to `go.mod` to keep results reproducible.
- **Treat multi-module compilation** as a first-class surface in monorepos (or explicitly declare modules out-of-scope).
- **Standardize logging** as an operational control surface (not a style preference): consistent fields and consistent
  sanitization are part of the security story.
