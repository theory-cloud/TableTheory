# Getting Started (Python)

<!-- AI Training: This is the getting-started guide for the TableTheory Python SDK -->
**Goal:** install `tabletheory-py`, connect to DynamoDB (AWS or DynamoDB Local), and perform your first CRUD operations with a model definition that is compatible with cross-language contracts.

## Prerequisites

- Python **3.14+**
- AWS credentials (for AWS) or DynamoDB Local
- Basic DynamoDB concepts (PK/SK, GSIs, condition expressions)

## Installation

This repo does **not** publish to PyPI. GitHub Releases are the source of truth.

### Option A: Install from GitHub Release assets (recommended for consumers)

**Stable (replace `X.Y.Z`):**

```bash
pip install \
  https://github.com/theory-cloud/tabletheory/releases/download/vX.Y.Z/theorydb_py-X.Y.Z-py3-none-any.whl
```

**Prerelease (replace `X.Y.Z-rc.N`):**

Python packages use PEP 440 prerelease formatting. Example: Git tag `v1.2.1-rc.1` becomes Python version `1.2.1rc1`,
so the wheel name is `theorydb_py-1.2.1rc1-...whl`.

```bash
pip install \
  https://github.com/theory-cloud/tabletheory/releases/download/vX.Y.Z-rc.N/theorydb_py-X.Y.ZrcN-py3-none-any.whl
```

### Option B: Develop from source (this monorepo)

```bash
# from repo root
uv --directory py sync --all-extras
```

## Quickstart (DynamoDB Local)

Start DynamoDB Local from the repo root:

```bash
make docker-up
```

Minimal example:

```python
from dataclasses import dataclass
import os

import boto3

from theorydb_py import ModelDefinition, Table, theorydb_field


@dataclass(frozen=True)
class Note:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    value: int = theorydb_field()


client = boto3.client(
    "dynamodb",
    endpoint_url=os.environ.get("DYNAMODB_ENDPOINT", "http://localhost:8000"),
    region_name=os.environ.get("AWS_REGION", "us-east-1"),
    aws_access_key_id=os.environ.get("AWS_ACCESS_KEY_ID", "dummy"),
    aws_secret_access_key=os.environ.get("AWS_SECRET_ACCESS_KEY", "dummy"),
)

model = ModelDefinition.from_dataclass(Note, table_name="notes_contract")
table = Table(model, client=client)

table.put(Note(pk="NOTE#1", sk="v1", value=123))
note = table.get("NOTE#1", "v1")
table.delete("NOTE#1", "v1")
```

## Next Steps

- Read [Core Patterns](./core-patterns.md) for cursor pagination, batch, transactions, streams, and encryption.
- Use [Testing Guide](./testing-guide.md) for strict fakes and deterministic encryption tests.
- Use [API Reference](./api-reference.md) when you need signature-level detail.

