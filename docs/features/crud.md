---
title: CRUD & Marshaling
description: How TableTheory marshals models to DynamoDB attributes — the foundational P0 contract.
---

# CRUD & Marshaling

The CRUD scenario is the foundation of the P0 contract: every TableTheory runtime must produce **identical DynamoDB items** for identical inputs and read them back identically. If Go writes a `Note` and Python reads it, the two runtimes see the same field values, the same attribute names, and the same DynamoDB types.

The canonical scenario lives at [`contract-tests/scenarios/p0/01-crud-basic.yml`](https://github.com/theory-cloud/tabletheory/blob/main/contract-tests/scenarios/p0/01-crud-basic.yml) and is exercised by every runtime on every commit.

## The contract

Given a model with `pk`, `sk`, one user attribute, and the same DMS shape across runtimes:

| Behavior                                  | Required across Go / TS / Python |
|-------------------------------------------|----------------------------------|
| Create writes all populated attributes     | yes                              |
| Zero values without `omit_empty` are written | yes                            |
| Attribute names match the DMS shape        | yes                              |
| Read returns identical field values        | yes                              |
| Update mutates only the named fields       | yes                              |
| Delete removes the item                    | yes                              |
| Missing item on Get raises a typed not-found error, never returns silent nil/None | yes |

## Go

```go
type Note struct {
    PK   string `theorydb:"pk" json:"pk"`
    SK   string `theorydb:"sk" json:"sk"`
    Body string `json:"body"`
}

// Create
db.Model(&Note{PK: "USER#1", SK: "NOTE#welcome", Body: "Hi."}).Create()

// Read
var got Note
db.Model(&Note{PK: "USER#1", SK: "NOTE#welcome"}).First(&got)

// Update specific fields
db.Model(&Note{PK: "USER#1", SK: "NOTE#welcome", Body: "Edited."}).Update("body")

// Delete
db.Model(&Note{PK: "USER#1", SK: "NOTE#welcome"}).Delete()
```

## TypeScript

```typescript
// TheorydbClient.update requires a version role on the model and the
// current version in the item payload. Most "real" TableTheory models
// declare version anyway; show it here so the CRUD flow is complete.
const Note = defineModel({
  name: 'Note',
  table: { name: 'notes_contract' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort:      { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK',      type: 'S', roles: ['pk'] },
    { attribute: 'SK',      type: 'S', roles: ['sk'] },
    { attribute: 'body',    type: 'S', optional: true, omit_empty: true },
    { attribute: 'version', type: 'N', roles: ['version'] },
  ],
});

const db = new TheorydbClient(ddb).register(Note);

await db.create('Note', { PK: 'USER#1', SK: 'NOTE#welcome', body: 'Hi.' });
const item = await db.get('Note', { PK: 'USER#1', SK: 'NOTE#welcome' });
// Pass the current version through; the runtime asserts it matches.
await db.update(
  'Note',
  { PK: 'USER#1', SK: 'NOTE#welcome', body: 'Edited.', version: item.version },
  ['body'],
);
await db.delete('Note', { PK: 'USER#1', SK: 'NOTE#welcome' });
```

## Python

```python
@dataclass(frozen=True)
class Note:
    pk:   str = theorydb_field(roles=["pk"])
    sk:   str = theorydb_field(roles=["sk"])
    body: str = theorydb_field(omitempty=True)

model = ModelDefinition.from_dataclass(Note, table_name="notes_contract")
table = Table(model, client=client)

table.put(Note(pk="USER#1", sk="NOTE#welcome", body="Hi."))
note = table.get("USER#1", "NOTE#welcome")
table.update("USER#1", "NOTE#welcome", {"body": "Edited."})  # third arg is a Mapping
table.delete("USER#1", "NOTE#welcome")
```

## Omit-empty subtlety

Without `omit_empty`, **zero values are written**: `""`, `0`, `False`/`false`, empty slices, empty maps. With `omit_empty`, the attribute is *omitted entirely* from the DynamoDB item.

This matters because DynamoDB distinguishes "attribute present with empty string" from "attribute absent" in queries and conditional expressions. The contract pins which behavior each combination produces, and every runtime must agree.

The dedicated [omit-empty scenario](https://github.com/theory-cloud/tabletheory/blob/main/contract-tests/scenarios/p0/02-omitempty.yml) exercises every type variant in the matrix.

## Related

- [Struct Definition Guide](../struct-definition-guide.md) — every tag and field shape
- [Core Patterns](../core-patterns.md) — query and transaction recipes built on top of CRUD
- [Contract Scenarios](../reference/contract-scenarios.md) — full P0 specification
