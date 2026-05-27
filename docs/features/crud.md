---
title: CRUD & Marshaling
description: How TableTheory marshals models to DynamoDB attributes — the foundational P0 contract.
---

# CRUD & Marshaling

The CRUD scenario is the foundation of the P0 contract: every TableTheory runtime must produce **identical DynamoDB items** for identical inputs and read them back identically. If Go writes a `Note` and Python reads it, the two runtimes see the same field values, the same attribute names, and the same DynamoDB types.

The canonical scenario lives at [`contract-tests/scenarios/crud/`](https://github.com/theory-cloud/tabletheory/tree/main/contract-tests/scenarios) and is exercised by every runtime on every commit.

## The contract

Given a model with `pk`, `sk`, one user attribute, and a naming strategy:

| Behavior                                  | Required across Go / TS / Python |
|-------------------------------------------|----------------------------------|
| `Put` writes all populated attributes      | yes                              |
| Zero values without `omitempty` are written | yes                              |
| Attribute names follow the naming strategy | yes                              |
| `Get` returns identical field values       | yes                              |
| `Update` mutates only changed attributes   | yes                              |
| `Delete` removes the item                  | yes                              |
| Missing item on `Get` returns a typed "not found" error, never `nil`/`None` silently | yes |

## Go

```go
type Note struct {
    _  struct{} `theorydb:"naming:snake_case"`
    PK string   `theorydb:"pk"  json:"pk"`
    SK string   `theorydb:"sk"  json:"sk"`

    Body string `theorydb:"omitempty" json:"body,omitempty"`
}

_ = db.Put(ctx, &Note{PK: "user#1", SK: "note#welcome", Body: "Hi."})

var n Note
_ = db.Get(ctx, &n, "user#1", "note#welcome")
```

## TypeScript

```typescript
@model({ naming: "snake_case", table: "notes" })
class Note {
  @field({ role: "pk" }) pk!: string;
  @field({ role: "sk" }) sk!: string;
  @field({ omitempty: true }) body?: string;
}

await db.put(new Note({ pk: "user#1", sk: "note#welcome", body: "Hi." }));
const n = await db.get(Note, "user#1", "note#welcome");
```

## Python

```python
@model(naming="snake_case", table="notes")
class Note:
    pk: str
    sk: str
    body: str | None = field(omitempty=True, default=None)

db.put(Note(pk="user#1", sk="note#welcome", body="Hi."))
n = db.get(Note, "user#1", "note#welcome")
```

## Omit-empty subtlety

Without `omitempty`, **zero values are written**: `""`, `0`, `False`/`false`, empty slices, empty maps. With `omitempty`, the attribute is *omitted entirely* from the DynamoDB item.

This matters because DynamoDB distinguishes "attribute present with empty string" from "attribute absent" in queries and conditional expressions. The contract pins which behavior each tag combination produces, and every runtime must agree.

The dedicated [omit-empty scenario](https://github.com/theory-cloud/tabletheory/tree/main/contract-tests/scenarios) exercises every type variant in the matrix.

## Related

- [Struct Definition Guide](../struct-definition-guide.md) — every tag and field shape
- [Core Patterns](../core-patterns.md) — query and transaction recipes built on top of CRUD
- [Contract Scenarios](../reference/contract-scenarios.md) — full P0 specification
