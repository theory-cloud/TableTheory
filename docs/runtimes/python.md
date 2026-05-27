---
title: Python
description: TableTheory for Python — installation, Lambda init, and the canonical model shape.
---

# Python runtime

The Python runtime lives under [`py/`](https://github.com/theory-cloud/tabletheory/tree/main/py) and is distributed as `tabletheory-py` / `theorydb_py`. It targets **Python 3.14** and the AWS SDK for Python (boto3).

Like the TypeScript runtime, Python is a **peer implementation** of the TableTheory contract — not a port. It passes the same P0 contract scenarios as Go and TypeScript or it is broken.

## Install

Distributed via immutable [GitHub Releases](https://github.com/theory-cloud/tabletheory/releases) — install the release wheel/sdist:

```bash
# Download the wheel from a GitHub Release, then:
pip install ./tabletheory_py-1.x.y-py3-none-any.whl
```

There is **no** PyPI publish. Distribution is GitHub Releases only across all three runtimes.

## Lambda init

```python
from tabletheory_py import TableTheory, model, field

@model(naming="snake_case", table="notes")
class Note:
    pk: str = field(role="pk")
    sk: str = field(role="sk")

    body: str | None = field(omitempty=True, default=None)

    version:    int = field(role="version")
    created_at: int = field(role="created_at")
    updated_at: int = field(role="updated_at")

# Module scope — reused across Lambda invocations.
db = TableTheory.lambda_init(
    table="notes",
    models=[Note],
)

def handler(event, _context):
    db.put(Note(**event))
```

> Always construct the client at module scope. The `lambda_init` entry point exists so the connection, the model registry, and the KMS configuration are built once and reused — moving any of those into the handler defeats the whole purpose.

## Tag vocabulary mapping

Python uses `field(...)` descriptors that map one-to-one onto the canonical `theorydb:` tag vocabulary.

| Go tag                       | Python field arg                    |
|------------------------------|-------------------------------------|
| `theorydb:"pk"`              | `field(role="pk")`                  |
| `theorydb:"sk"`              | `field(role="sk")`                  |
| `theorydb:"gsi1pk"`          | `field(role="gsi1pk")`              |
| `theorydb:"encrypted"`       | `field(encrypted=True)`             |
| `theorydb:"version"`         | `field(role="version")`             |
| `theorydb:"created_at"`      | `field(role="created_at")`          |
| `theorydb:"updated_at"`      | `field(role="updated_at")`          |
| `theorydb:"ttl"`             | `field(role="ttl")`                 |
| `theorydb:"omitempty"`       | `field(omitempty=True)`             |
| `theorydb:"naming:snake_case"` | `@model(naming="snake_case")`     |

## Workflows

- `cd py && python -m unittest` — full unit suite (stdlib `unittest`)
- `cd py && ruff check --line-length 120 .` — lint
- The Python runtime is exercised against shared contract scenarios via [`contract-tests/runners/py/`](https://github.com/theory-cloud/tabletheory/tree/main/contract-tests/runners) on every commit

## Where to go next

- [Getting Started](../getting-started/) — full walkthrough
- [Struct Definition Guide](../struct-definition-guide/) — model authoring
- [Features → CRUD & Marshaling](../features/crud/), [Optimistic Locking](../features/optimistic-locking/), [Encryption](../features/encryption/)
- [`py/docs/`](https://github.com/theory-cloud/tabletheory/tree/main/py/docs) on GitHub — Python-specific runtime documentation

## Stability and support

The Python runtime is **GA**. Versions are aligned with Go and TypeScript per the multi-language version-sync invariant (see [`scripts/verify-branch-version-sync.sh`](https://github.com/theory-cloud/tabletheory/blob/main/scripts/verify-branch-version-sync.sh)).
