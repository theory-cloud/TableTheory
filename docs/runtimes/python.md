---
title: Python
description: TableTheory for Python — installation, ModelDefinition + Table + theorydb_field.
---

# Python runtime

The Python runtime lives under [`py/`](https://github.com/theory-cloud/tabletheory/tree/main/py) and is published as the wheel `tabletheory_py-X.Y.Z-py3-none-any.whl`. The canonical importable module inside that wheel is **`tabletheory_py`**. It targets **Python 3.12+** and the AWS SDK for Python (`boto3`).

The Python runtime is a **peer** implementation of the TableTheory contract — not a port. It passes the same P0 contract scenarios as Go and TypeScript independently.

## Install

This repo does **not** publish to PyPI. GitHub Releases are the source of truth. Install the release wheel directly:

```bash
# Stable release (replace X.Y.Z)
pip install \
  https://github.com/theory-cloud/tabletheory/releases/download/vX.Y.Z/tabletheory_py-X.Y.Z-py3-none-any.whl

# Prerelease (replace X.Y.Z-rc.N — PEP 440 form: vX.Y.Z-rc.N tag → X.Y.ZrcN wheel)
pip install \
  https://github.com/theory-cloud/tabletheory/releases/download/vX.Y.Z-rc.N/tabletheory_py-X.Y.ZrcN-py3-none-any.whl
```

The **release wheel asset** is `tabletheory_py-X.Y.Z-py3-none-any.whl`; the **canonical importable module** inside that wheel is `tabletheory_py`. Use `from tabletheory_py import …` in your application code. The legacy `theorydb_py` import path remains available for existing consumers.

### Find and automate release updates

GitHub Releases are the version source of truth:

```bash
gh release view --repo theory-cloud/TableTheory --json tagName,publishedAt,url
gh release list --repo theory-cloud/TableTheory --exclude-drafts --limit 10
```

Copy this Renovate regex manager into the consuming repository's `renovate.json` to update stable Python wheel URLs:

```json
{
  "customManagers": [
    {
      "customType": "regex",
      "description": "Update TableTheory Python wheel GitHub Release asset URLs",
      "managerFilePatterns": ["/(^|/)requirements.*\\.txt$/", "/(^|/)README\\.md$/", "/(^|/)docs/.+\\.md$/"],
      "matchStrings": [
        "https://github\\.com/theory-cloud/[Tt]able[Tt]heory/releases/download/v(?<currentValue>\\d+\\.\\d+\\.\\d+)/tabletheory_py-(?<assetVersion>\\d+\\.\\d+\\.\\d+)-py3-none-any\\.whl"
      ],
      "datasourceTemplate": "github-releases",
      "depNameTemplate": "theory-cloud/TableTheory",
      "versioningTemplate": "semver",
      "extractVersionTemplate": "^v(?<version>\\d+\\.\\d+\\.\\d+)$",
      "autoReplaceStringTemplate": "https://github.com/theory-cloud/TableTheory/releases/download/v{{{newValue}}}/tabletheory_py-{{{newValue}}}-py3-none-any.whl"
    }
  ]
}
```

For the combined TypeScript + Python config, see [Consumer update automation](../guides/consumer-updates.md).

## ModelDefinition + Table

The Python public surface declares models as plain dataclasses with `theorydb_field()` defaults, then registers them via `ModelDefinition.from_dataclass()`. CRUD runs through a `Table` instance.

```python
from dataclasses import dataclass

import boto3
from tabletheory_py import ModelDefinition, Table, theorydb_field


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

`theorydb_field(roles=[…], encrypted=…, omitempty=…)` maps one-to-one onto the canonical TableTheory contract:

| Go tag                  | Python field arg                       |
| ----------------------- | -------------------------------------- |
| `theorydb:"pk"`         | `theorydb_field(roles=["pk"])`         |
| `theorydb:"sk"`         | `theorydb_field(roles=["sk"])`         |
| `theorydb:"gsi1pk"`     | `theorydb_field(roles=["gsi1pk"])`     |
| `theorydb:"encrypted"`  | `theorydb_field(encrypted=True)`       |
| `theorydb:"version"`    | `theorydb_field(roles=["version"])`    |
| `theorydb:"created_at"` | `theorydb_field(roles=["created_at"])` |
| `theorydb:"updated_at"` | `theorydb_field(roles=["updated_at"])` |
| `theorydb:"ttl"`        | `theorydb_field(roles=["ttl"])`        |
| `theorydb:"omitempty"`  | `theorydb_field(omitempty=True)`       |

## CRUD methods

`Table` exposes the canonical CRUD surface:

```python
table.put(note)
note = table.get(pk, sk)                            # raises NotFoundError on miss
table.update(pk, sk, {"body": "updated"})           # third arg is a Mapping[str, Any]
table.update(pk, sk, {"body": "guarded"}, expected_version=note.version)  # versioned write
table.delete(pk, sk)

# Composite updates use update_builder for set/remove/add chains.
table.update_builder(pk, sk).set("body", "updated").execute()
```

## Aggregation helper warning

Python aggregation helpers (`sum_field`, `average_field`, `min_field`, `max_field`, `aggregate_field`, `count_distinct`,
and `group_by`) are client-side conveniences over already materialized sequences. If those sequences come from
`query_all`, `scan_all`, or `scan_all_segments`, the runtime has loaded every matching item into memory before computing
the result. Use them only for bounded result sets. When the planned native `count()` API lands, use it instead for
count-only access patterns.

## Workflows

- `uv --directory py run ruff format --check .` — format verification
- `uv --directory py run ruff check .` — lint using the `py/pyproject.toml` line length of 110
- `uv --directory py run mypy src` — typecheck
- `uv --directory py run pytest -q tests/unit` — unit pytest suite
- `uv --directory py run pytest -q tests/integration` — integration tests with DynamoDB Local
- Exercised against shared contract scenarios via [`contract-tests/runners/`](https://github.com/theory-cloud/tabletheory/tree/main/contract-tests/runners) on every commit

## Where to go next

- [Getting Started](https://tabletheory.theorycloud.ai/getting-started/) — full walkthrough
- [`py/docs/getting-started.md`](https://github.com/theory-cloud/tabletheory/blob/main/py/docs/getting-started.md) — Python-specific runtime documentation
- [`py/docs/api-reference.md`](https://github.com/theory-cloud/tabletheory/blob/main/py/docs/api-reference.md) — full Python API reference
- [Features → CRUD & Marshaling](../features/crud.md), [Optimistic Locking](../features/optimistic-locking.md), [Encryption](../features/encryption.md)

## Stability and support

The Python runtime is **GA**. Versions are aligned with Go and TypeScript per the multi-language version-sync invariant (see [`scripts/verify-branch-version-sync.sh`](https://github.com/theory-cloud/tabletheory/blob/main/scripts/verify-branch-version-sync.sh)).
