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
In Go transaction puts, explicitly non-nil empty collections inside nested structs are preserved unless those fields
use `omitempty`.

The dedicated [omit-empty scenario](https://github.com/theory-cloud/tabletheory/blob/main/contract-tests/scenarios/p0/02-omitempty.yml) exercises every type variant in the matrix.

### Nil and empty collections per writer path

DynamoDB has no "nil" collection: an attribute holds an `L`/`M`, holds `NULL`, or is absent. TableTheory's runtimes, and Go's two writers, store **different** representations for a nil collection. That split is a **permanent compatibility contract**: each writer path's stored representation is frozen by the operator's policy that TableTheory supports its consumers rather than changing stored data, no writer's output may change, and the rows below are asserted by tests.

For a collection written **without** `omit_empty`:

| Input (logical class) | Go `Create` / `BatchCreate` / `BatchWrite` | Go `Transact().Put` / `Create` | TypeScript `create` / `save` / `batchWrite` / `transactWrite` | Python `put` / `save` / `batch_write` / `transact_write` |
|---|---|---|---|---|
| nil list or map | `NULL` | `L` or `M` | attribute absent | `NULL` |
| non-nil empty list or map | `L` or `M` | `L` or `M` | `L` or `M` | `L` or `M` |
| nil list or map, `omit_empty` | absent | absent | absent | absent |
| empty list or map, `omit_empty` | absent | absent | absent | absent |
| `false` or `0` | `BOOL` or `N` | `BOOL` or `N` | `BOOL` or `N` | `BOOL` or `N` |

Three further differences belong to the same table:

- **TypeScript never stores `NULL` for a missing collection.** It has no nil collection, so its nearest input is an absent property, which the marshaler drops. A collection nested inside an `M` value can instead be an explicit `null`, which TypeScript does store as `NULL`.
- **Python stores `None` as `NULL` on every writer path**, which matches Go's put family but not Go's `TransactWrite`.
- **Go's `TransactWrite` ignores the `set` tag.** A set-tagged slice is stored as an empty `L` there, where the put family stores `NULL`.

The single source of truth for these rows is [`contract-tests/serialization/nil-empty-collections.yml`](https://github.com/theory-cloud/tabletheory/blob/main/contract-tests/serialization/nil-empty-collections.yml). Each runtime's test suite loads that one file and asserts its own column against payloads captured from its real writers, and each asserts a derived row count, so a change to any writer for these classes — or a dropped row — fails the build.

### Measuring what a writer will store

Because the writer paths store different representations for some inputs, the same model can be measured two ways. Each runtime exposes one item-serialization preflight that returns exactly the DynamoDB item a named writer path stores:

| Runtime | Preflight | Writer paths |
|---|---|---|
| Go | `marshal.MarshalItemForWriterPath(model, metadata, path)` | `marshal.ItemWriterPathPut`, `marshal.ItemWriterPathTransactWrite` |
| TypeScript | `marshalItemForWriterPath(model, item, path)` | `'put'`, `'transact_write'` |
| Python | `marshal_item_for_writer_path(table, item, path)` | `ItemWriterPath.PUT`, `ItemWriterPath.TRANSACT_WRITE` |

Both paths return the same representation in TypeScript and Python, which funnel every whole item through one serializer. Go needs the parameter because its two writers really do differ. A preflight refuses a model that declares `encrypted` attributes, because the stored envelope depends on KMS material a static preflight cannot see. It has no side effects, contacts nothing, and changes nothing that is written.

### Custom type converters

A model that registers a **custom type converter** is not covered by the preflight above: the converter replaces
TableTheory's own value handling, so the preflight cannot predict what it stores. In Go a converter is registered on the
database (`RegisterTypeConverter`), for example for `map[string]interface{}`; in TypeScript and Python it is a
per-attribute `converter` on the attribute definition.

A read-only audit of the current consumer set (Autheory, theory-mcp-server, theorycloud-ai, FaceTheory and AppTheory at
`origin/staging`) found **no consumer on TableTheory v5 that registers a converter for a slice, map or pointer**. The only
converters registered in that set are Autheory's two Go converters — for `map[string]interface{}` and for `interface{}`,
both in `pkg/autheory/repository/dynamorm_converters.go` — and Autheory pins TableTheory v4. A converter is shared by
every writer path rather than being per-writer, so it has no nil/empty writer split to pin. That is why the shared table
above has no custom-converter rows.

### Explicit empty updates

DMS v0.2 distinguishes an unselected field from a selected empty field:

- an unselected empty `omit_empty` field leaves the persisted attribute
  unchanged;
- an explicitly selected empty `omit_empty` field removes the persisted
  DynamoDB attribute;
- a selected field without `omit_empty` stores its empty value.

This rule is identical in Go, TypeScript, and Python. Consumers that need a
present empty attribute must not mark that field `omit_empty`, or must use the
runtime's deliberate low-level `SET` expression builder.

## Related

- [Struct Definition Guide](../struct-definition-guide.md) — every tag and field shape
- [Core Patterns](../core-patterns.md) — query and transaction recipes built on top of CRUD
- [Contract Scenarios](../reference/contract-scenarios.md) — full P0 specification
