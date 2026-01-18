# Migration Guide (Python)

This guide helps migrate from raw boto3 DynamoDB usage to `theorydb_py`.

## Migration: raw `put_item` → `table.put`

### Problem
Hand-authored `client.put_item(...)` payloads are verbose and drift-prone across services.

### Solution
Define a model once and use typed helpers.

```python
table = Table(model, client=client)
table.put(Note(pk="A", sk="1", value=123))
```

