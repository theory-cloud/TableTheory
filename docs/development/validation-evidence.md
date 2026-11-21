# Validation evidence

TableTheory's gates run **once, locally, on a production host**. A records-only
commit binds each result to the exact input digests it was produced against. CI
on the pull request into `staging` **verifies** that record; it never executes a
gate.

The machine-readable half of this policy is `scripts/evidence/validation-gates.json`.
`scripts/evidence/check-policy.py` fails when the two drift apart, and it runs
inside the rubric (COM-6), so a policy edit that leaves the documentation behind
cannot merge.

## What "passing" means

- An item passes when its verifier result is in a locally produced, input-bound
  evidence record that CI verifies on the pull request to `staging`.
- The record binds a **declared input scope** per gate: the tracked file digests
  inside that scope, the gate's approved command set, and the **proof profile**
  (Go pin, Node major, Python major.minor, platform) the claim belongs to.
- CI verifies that the record still applies to the pull-request tree: no gate executes in CI.
  Verification recomputes the input projections and compares them with the
  record.
- A missing, stale, mismatched or failed record fails the job and prints
  **produce evidence locally** with the exact command that produces one.
- Records are unsigned. The controls against a hand-edited record are paired
  review of the records-only commit's diff plus the catch-all rubric record, whose
  scope is every path, so an edit to verifier tooling cannot leave a record
  looking current.

## Gates

| Gate | Proof profiles | Approved commands |
|---|---|---|
| `rubric` | `default` | `make rubric` (composes the recorded product gates) |
| `unit` | `default` | `bash scripts/verify-unit-tests.sh` |
| `integration` | `default` | `bash scripts/verify-integration-tests.sh` |
| `coverage` | `default` | `make unit-cover`; `bash scripts/verify-coverage.sh` |
| `format` | `default` | `bash scripts/verify-formatting.sh` |
| `lint` | `default` | `bash scripts/verify-lint.sh` |
| `builds` | `default` | `bash scripts/verify-builds.sh` |
| `ts` | `default`, `floor-node22` | deps; `npm --prefix ts run check`; coverage; `test:integration` |
| `py` | `default`, `floor-py3.12` | `uv sync`; `scripts/verify-python-static.sh`; coverage; integration |
| `go-integration` | `linux-arm64` | `bash scripts/verify-go-integration.sh` |
| `examples-local` | `default` | `make examples-local` |

`rubric` is the catch-all: its scope is `**`, so any edit to tracked content —
including the evidence tooling under `scripts/` and `gov-infra/` — stales the
record the staging pull request verifies.

## Release history is not a product input

A generated release moves exactly two tracked files: `.release-please-manifest.json`
and `CHANGELOG.md`. Neither carries product bytes, so neither can change a claim,
and binding them as product input made every `**`-scoped record go stale on every
release and every `main` → `staging` back-merge.

The registry's **`release-notes`** class narrows only those two paths. A changelog
contributes its path and mode and never its bytes, so a generated section, a hand
edit and a reordering project identically. The release-please manifest contributes
its structure with the version values excluded, so a version bump projects
identically while a structural edit still fails closed. Every other release-only
path keeps version-literal normalization only, and a product byte still stales the
records whose scope covers it.

`scripts/evidence/check-policy.py` fixes the reviewed allowlist for the class and
requires the registry to match it exactly. It rejects a prefix, a glob or a
directory entry, a path that is not release-only, a path inside a product-bytes
prefix, a path that is also a dependency-scan input, and a path that is not a
tracked file, so the class cannot grow silently.

## What CI verifies

`quality-gates.yml` verifies one record per step. Every declared gate profile has
exactly one verification path, and `scripts/evidence/check-policy.py` fails when
a declared profile has none:

| Step | Record | Path |
|---|---|---|
| `rubric` | `rubric/default` | direct |
| `unit`, `integration`, `coverage`, `format`, `lint`, `builds` | `<gate>/default` | composed by the verified `rubric` record |
| `ts` | `ts/default` | direct |
| `py` | `py/default` | direct |
| `examples-local` | `examples-local/default` | direct |
| `ts` | `ts/floor-node22` | direct (Linux arm64 floor) |
| `py` | `py/floor-py3.12` | direct (Linux arm64 floor) |
| `go-integration` | `go-integration/linux-arm64` | direct (Linux arm64 floor) |

A pair that is **produced but unverified** — declared by a gate, no verification
step, no verified gate composing it — fails the policy, so a record nothing reads
cannot sit in the tree. A verification step that names a gate or profile the
registry does not declare fails the policy too, rather than turning the required
check red after the push.

The integration and floor producers start DynamoDB Local from
`amazon/dynamodb-local:3.1.0@sha256:7ef4a2c4…7752`. A tag is mutable, so the
digest is what binds the evidence to the binary the gate executed;
`scripts/verify-dynamodb-local-pin.sh` (COM-7) fails on any unpinned reference in
a compose file, Makefile, shell script, container template or checked-in profile.

## Proof profiles and production hosts

A **proof profile** is a tracked declaration of a supported environment. The
collector probes the host it runs on and refuses to record a profile that host
does not satisfy, so a receipt's environment is always the real probe. A
platform-bound claim that cannot be produced on the default host has an explicit
fallback profile and a digest-pinned Linux producer.

| Profile | Go | Node | Python | Platform | Produced by |
|---|---|---|---|---|---|
| `default` | go1.27.4 | 24 | 3.14 | darwin/arm64 | the approved macOS production host |
| `floor-node22` | go1.27.4 | 22 | 3.14 | linux/arm64 | `node:22-bookworm@sha256:f796bda8…0ee3` |
| `floor-py3.12` | go1.27.4 | 24 | 3.12 | linux/arm64 | `python:3.12-bookworm@sha256:3dc01f34…904e` |
| `linux-arm64` | go1.27.4 | 24 | 3.14 | linux/arm64 | `golang:1.27-bookworm@sha256:8d18482d…5da9` |

Floor records are produced in a container and verified in CI under their own
profile name, so a changed floor or a changed input invalidates only that proof.
`scripts/evidence/floor-run.sh` runs the collector inside the digest-pinned
container for a floor profile: no environment is ever asserted by hand.

`collect.sh --all` covers every gate this host can produce, that is, every gate
that declares a `default` profile. A gate that declares only a floor profile (the
Linux Go integration floor) is skipped with the command that collects it, and CI
verifies it explicitly, so skipping it never turns a missing floor record into a
pass.

## Producing and verifying

```bash
# Produce every affected gate for the default profile (incremental).
scripts/evidence/collect.sh --all

# Produce one gate, on the approved production host.
scripts/evidence/collect.sh --gate unit

# Produce a floor record inside its digest-pinned Linux producer.
scripts/evidence/floor-run.sh --profile floor-py3.12 --gate py --clean-host-state
scripts/evidence/floor-run.sh --profile floor-node22 --gate ts --clean-host-state
scripts/evidence/floor-run.sh --profile linux-arm64 --gate go-integration \
  --dynamodb-endpoint http://host.docker.internal:<port>

# Verify exactly what CI verifies, without executing anything.
scripts/evidence/verify-gate.sh --gate rubric --profile default
```

Collection is incremental. `collect.sh --all` enumerates the required coverage
and schedules only missing, failed, stale or affected gates; a second run over
unchanged inputs executes **zero commands**. `--force` is the explicit
re-execution override and is never the default. `--dry-run` prints the schedule
without executing it.

Each gate declares its input scope, its dependencies and its **resource group**
(`cpu`, `repo-write`, `docker`, `network`), so independent gates overlap while
conflicting build, filesystem and container work serializes. Bounded workers are
the scheduler's default; `--jobs` overrides it within a tracked bound.

A `--profile` names the proof profile to verify. The gate's first declared
profile is the default; a **fallback profile** is tried in declaration order only
when the requested one cannot be applied.

## Composition instead of re-execution

The rubric composes the product gates instead of running their commands a second
time. Each composed gate's script calls
`scripts/evidence/reuse-if-recorded.sh <gate>`: when the reviewed record applies
to these exact inputs, the script exits without executing anything, and the
rubric records that attribution. When the record does not apply — stale, missing,
failed, or produced for another profile — the script runs its real command.

## The execution matrix

| Leg | Event |
|---|---|
| `pr-staging` | pull request into `staging`: verify the record, execute no gate |
| `push-staging` | merge push: publish side effects only (pages deploy, subtree publish) |
| `pr-staging-premain` | promotion pull request: release hygiene only |
| `push-premain` | release-please generation and publish only |
| `release-pr-premain` | generated RC pull request: release hygiene only |
| `release-merge-premain` | RC merge: publish only |
| `pr-premain-main` | promotion pull request: release hygiene only |
| `push-main` | release-please generation and publish only |
| `release-pr-main` | generated stable pull request: release hygiene only |
| `release-merge-main` | stable merge: publish only |
| `backmerge-main-staging` | records-only refresh, then the staging verification set |

## Accepted audit findings

There are currently no accepted npm-audit findings in `examples/cdk-multilang`.
The `aws-cdk-lib@2.278.0` bump retired the GHSA-g3hv-75wg-m249 pair (the bundled
minimatch leaf and npm's derived `aws-cdk-lib` parent entry); the 2.278.0 release
bundles `minimatch@11.0.9` and `brace-expansion@5.0.13`, so
`gov-infra/planning/theorydb-visible-npm-audit-findings.json` is empty.

The rubric gate no longer sets `NPM_AUDIT_ACCEPT_VISIBLE_FINDINGS`, so no gate
run silently accepts a visible finding. A future upstream-bundled regression must
be handled by a reviewed visible entry (or an upgrade), never by a silent
acceptance.

## Records-only commits

A records-only commit contains the record files under
`gov-infra/evidence/validation/<gate>/<profile>.json` and nothing else. That is a
review control: the reviewer reads the commit's diff, and the paired review
covers the claim that the record is a real local result for that tree. The raw
command logs beside the records stay untracked.

## When CI says "produce evidence locally"

1. Check out the exact head the pull request carries.
2. Run the printed command, or `scripts/evidence/collect.sh --all` for the
   default profile plus the floor commands above for the floors that changed.
3. Commit only the record files.
4. Push the records-only commit. CI verifies again; nothing re-executes.

A generated release pull request and a release-merge push never verify a product
record: they carry release hygiene and publish work only.
