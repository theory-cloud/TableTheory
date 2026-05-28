---
title: Python
description: TableTheory for Python — installation, ModelDefinition + Table + theorydb_field.
---

# Python runtime

The Python runtime lives under [`py/`](https://github.com/theory-cloud/tabletheory/tree/main/py) and is distributed as the wheel `theorydb_py`. It targets **Python 3.14** and the AWS SDK for Python (`boto3`).

The Python runtime is a **peer** implementation of the TableTheory contract — not a port. It passes the same P0 contract scenarios as Go and TypeScript independently.

## Install

This repo does **not** publish to PyPI. GitHub Releases are the source of truth. Install the release wheel directly:

```bash
# Stable release (replace X.Y.Z)
pip install \
  https://github.com/theory-cloud/tabletheory/releases/download/vX.Y.Z/theorydb_py-X.Y.Z-py3-none-any.whl

# Prerelease (replace X.Y.Z-rc.N — PEP 440 form: vX.Y.Z-rc.N tag → X.Y.ZrcN wheel)
pip install \
  https://github.com/theory-cloud/tabletheory/releases/download/vX.Y.Z-rc.N/theorydb_py-X.Y.ZrcN-py3-none-any.whl
```

The **importable module name is `theorydb_py`**, not `tabletheory_py`. The release wheel filename follows the same module name.

## ModelDefinition + Table

The Python public surface declares models as plain dataclasses with `theorydb_field()` defaults, then registers them via `ModelDefinition.from_dataclass()`. CRUD runs through a `Table` instance.

```python
from dataclasses import dataclass

import boto3
from theorydb_py import ModelDefinition, Table, theorydb_field


@dataclass(frozen=True)
class Note:
    pk:   str = theorydb_field(roles=["pk"])
    sk:   str = theorydb_field(roles=["sk"])
    body: str = theorydb_field()


client = boto3.client("dynamodb", region_name="us-east-1")
model  = ModelDefinition.from_dataclass(Note, table_name="notes_contract")
table  = Table(model, client=client)

table.put(Note(pk="USER#42", sk="NOTE#welcome", body="Hello, Theory Cloud."))

note = table.get("USER#42", "NOTE#welcome")
table.delete("USER#42", "NOTE#welcome")
```

For a complete working program, see [`py/docs/getting-started.md`](https://github.com/theory-cloud/tabletheory/blob/main/py/docs/getting-started.md).

## Role vocabulary

`theorydb_field(roles=[…], encrypted=…, omit_empty=…)` maps one-to-one onto the canonical TableTheory contract:

| Go tag                       | Python field arg                          |
|------------------------------|-------------------------------------------|
| `theorydb:"pk"`              | `theorydb_field(roles=["pk"])`            |
| `theorydb:"sk"`              | `theorydb_field(roles=["sk"])`            |
| `theorydb:"gsi1pk"`          | `theorydb_field(roles=["gsi1pk"])`        |
| `theorydb:"encrypted"`       | `theorydb_field(encrypted=True)`          |
| `theorydb:"version"`         | `theorydb_field(roles=["version"])`       |
| `theorydb:"created_at"`      | `theorydb_field(roles=["created_at"])`    |
| `theorydb:"updated_at"`      | `theorydb_field(roles=["updated_at"])`    |
| `theorydb:"ttl"`             | `theorydb_field(roles=["ttl"])`           |
| `theorydb:"omitempty"`       | `theorydb_field(omit_empty=True)`         |

## CRUD methods

`Table` exposes the canonical CRUD surface:

```python
table.put(note)
note = table.get(pk, sk)                       # raises NotFound on miss
table.update(pk, sk, body="updated")
table.delete(pk, sk)

# Composite updates use the update_builder helper for set/remove/add/delete.
table.update_builder(pk, sk).set("body", "updated").commit()
```

## Workflows

- `cd py && python -m unittest` — full unit suite (stdlib `unittest`)
- `cd py && ruff check --line-length 120 .` — lint
- Exercised against shared contract scenarios via [`contract-tests/runners/`](https://github.com/theory-cloud/tabletheory/tree/main/contract-tests/runners) on every commit

## Where to go next

- [Getting Started](https://theory-cloud.github.io/tabletheory/getting-started/) — full walkthrough
- [`py/docs/getting-started.md`](https://github.com/theory-cloud/tabletheory/blob/main/py/docs/getting-started.md) — Python-specific runtime documentation
- [`py/docs/api-reference.md`](https://github.com/theory-cloud/tabletheory/blob/main/py/docs/api-reference.md) — full Python API reference
- [Features → CRUD & Marshaling](../features/crud.md), [Optimistic Locking](../features/optimistic-locking.md), [Encryption](../features/encryption.md)

## Stability and support

The Python runtime is **GA**. Versions are aligned with Go and TypeScript per the multi-language version-sync invariant (see [`scripts/verify-branch-version-sync.sh`](https://github.com/theory-cloud/tabletheory/blob/main/scripts/verify-branch-version-sync.sh)).
