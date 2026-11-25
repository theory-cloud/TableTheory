---
title: Runtime support
description: Supported Node.js and Python versions for TableTheory, the Node 22 deprecation, and the planned Node floor raise.
---

# Runtime support

TableTheory ships three peer runtimes — Go, TypeScript and Python — against one
contract. This page states which interpreter versions each language runtime is
**tested** on, which versions are **supported**, and which are being retired.

A version is *supported* when the consumer package declares it inside its floor
(`engines.node` for TypeScript, `requires-python` for Python) and the repository
proves that line. A version is *tested* when a record binds the package suite, its
integration suite and a clean-consumer install of the packed artifact to that
exact interpreter line. Supported and tested are different claims: this page
separates them on purpose.

## Supported lines

| Runtime | Floor (unchanged) | Supported and tested lines | Notes |
| --- | --- | --- | --- |
| TypeScript | `node >=22` | Node 22, Node 24, Node 26 | Node 22 is **deprecated** — see below |
| Python | `python >=3.12` | Python 3.12, 3.14, 3.15 | no line is deprecated |
| Go | module floor in `go.mod` | the pinned Go toolchain | Go has no separate support window here |

The **floor is the minimum a consumer may install on**; the middle and newest
lines are what the repository proves on every pull request into `staging`. Adding
a tested line never raises the floor by implication: a consumer on Node 22 or
Python 3.12 keeps working exactly as before.

These are **package and tooling compatibility claims**: the released artifacts
install and run on that interpreter. They are not Lambda runtime deployment
claims, and they do not change any Lambda runtime the repository's examples
declare — a deployed function's runtime is chosen separately, by its own
selector, under its own review.

Each supported line is proven by its own evidence record, produced on the real
interpreter and verified in the hosted quality gates:

- `ts/floor-node22` — Node 22 in a digest-pinned Linux arm64 container;
- `ts/default` — Node 24 on the production host;
- `ts/floor-node26` — Node 26 in a digest-pinned Linux arm64 container;
- `py/floor-py3.12` — Python 3.12 in a digest-pinned Linux arm64 container;
- `py/default` — Python 3.14 on the production host;
- `py/floor-py3.15` — Python 3.15 in a digest-pinned Linux arm64 container.

Each TypeScript and Python record ends with a clean-consumer install: the packed
tarball (or built wheel) is installed into an empty directory that holds nothing
from this repository, and it runs one real call path against DynamoDB Local. That
is the claim a release makes to a consumer, so it is the claim the record binds.
See [Validation evidence](https://github.com/theory-cloud/tabletheory/blob/main/docs/development/validation-evidence.md) for how a record
is produced and what CI verifies.

## Node 22 is deprecated

Node.js 22 reaches upstream end-of-life on **2027-04-30**. TableTheory announces
the deprecation now, in this release line, and keeps Node 22 **supported and
tested** until the floor moves:

- **Now (this release):** Node 22 works, is tested on every pull request, and stays
  inside the declared `engines.node: ">=22"` floor. The deprecation is a notice,
  not a behavior change.
- **2027-02 (planned):** a dated major release raises the floor to
  `engines.node: ">=24"` and stops proving Node 22. The raise happens roughly ten
  weeks before the upstream end-of-life so consumers have a release with a stable
  floor to move to before Node 22 stops receiving fixes.
- **The floor raise is a major version bump.** It arrives on its own schedule,
  with migration notes, not by surprise inside a minor release.

Nothing about Node 22 changes today: no API changes, no runtime behavior changes,
no dependency-floor change. The notice exists so that a consumer planning for
2027 knows what TableTheory will do and roughly when.

The Python runtime has no equivalent notice: Python 3.12, 3.14 and 3.15 are all
inside the unchanged `requires-python: ">=3.12"` floor.

## Migration notes

**Running on Node 26 or Python 3.15 for the first time.** Install the current
release exactly as documented — there is no new artifact, entry point or flag.
The packed TypeScript tarball and the Python wheel are the same assets on every
supported line; only the interpreter changes.

```bash
# TypeScript, on Node 26
npm install --save-exact \
  https://github.com/theory-cloud/tabletheory/releases/download/vX.Y.Z/theory-cloud-tabletheory-ts-X.Y.Z.tgz

# Python, on Python 3.15
pip install \
  https://github.com/theory-cloud/tabletheory/releases/download/vX.Y.Z/tabletheory_py-X.Y.Z-py3-none-any.whl
```

**Preparing for the Node 22 floor raise (2027-02).** Nothing is required today.
When the major lands, a consumer on Node 22 must move to Node 24 or newer before
upgrading past it. Consumers that pin a Node release in CI should add Node 26 to
their matrix now: it is the line TableTheory proves on every pull request, so a
failure there is a real incompatibility rather than an untested guess.

**What a deprecation does *not* mean.** It does not mean Node 22 is untested,
unsupported, or unsafe in this line. The deprecation marks the version as leaving
the window; the support boundary moves only when the major ships.

## Where to go next

- [TypeScript runtime](typescript.md) — install, `defineModel` + `TheorydbClient`
- [Python runtime](python.md) — install, `ModelDefinition` + `Table`
- [Go runtime](go.md) — struct tags and `LambdaDB`
- [Validation evidence](https://github.com/theory-cloud/tabletheory/blob/main/docs/development/validation-evidence.md) — how each line is proven
