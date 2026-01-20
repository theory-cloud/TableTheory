# FaceTheory ISR Cache Schema (TableTheory-Compatible)

This document defines a **recommended DynamoDB item shape** for FaceTheory’s ISR cache metadata and regeneration locks.
The intent is to standardize schema and semantics so multiple services (Go/TypeScript/Python) can interoperate without
re-inventing lock + metadata patterns.

## Recommended table shape

Use **one DynamoDB table** that stores two item types per cache key:

- **Metadata item**: `sk = "META"`
- **Lease/lock item**: `sk = "LOCK"`

This keeps all ISR-related coordination for a cache key in a single partition, while still allowing clean separation
between “what was generated” (metadata) and “who is regenerating” (lease).

## Key design

### Partition key (`pk`)

`pk` MUST uniquely identify the cache entry (and tenant/site if applicable).

Recommended format (examples):

- single-tenant: `pk = "CACHE#<cache_key_hash>"`
- multi-tenant: `pk = "TENANT#<tenant_id>#CACHE#<cache_key_hash>"`

Notes:

- Prefer a **stable hash** of long/variable URL-like keys (e.g., sha256 hex) to keep the partition key short and avoid
  surprising hot partitions caused by common prefixes.
- Keep tenant/site identifiers at the front so operational queries can still group by tenant when needed.

### Sort key (`sk`)

Use constant sort keys to model item roles:

- `META` for cache metadata
- `LOCK` for regeneration lease state

## Metadata item shape (`sk = "META"`)

Recommended attributes:

- `s3_key` (string): S3 object key for the generated body (HTML/JSON/etc).
- `generated_at` (number, epoch seconds): when the body was generated.
- `revalidate_seconds` (number): ISR interval (freshness window).
- `etag` (string, optional): strong or weak ETag for clients/CDN.
- `ttl` (number, epoch seconds, optional): best-effort GC horizon for old metadata.

Freshness rule (correctness boundary):

- **Fresh/stale is determined by** `generated_at + revalidate_seconds`, not by DynamoDB TTL.

## Lease item shape (`sk = "LOCK"`)

Recommended attributes:

- `lease_token` (string): random token identifying the lock owner (required for refresh/release).
- `lease_expires_at` (number, epoch seconds): lease expiry time.
- `ttl` (number, epoch seconds, optional): best-effort GC horizon for lock rows (>= `lease_expires_at` + buffer).

Lock rule (correctness boundary):

- A lease is considered **held** iff `lease_expires_at > now`.

## TTL strategy (do’s and don’ts)

- ✅ Use DynamoDB TTL as **garbage collection**, not as a correctness boundary (TTL deletion has lag).
- ✅ Use a safety buffer for TTL (minutes to hours) so readers don’t depend on exact deletion timing.
- ❌ Do not treat “item missing” as meaning “never existed”; TTL may delete later than expected.

## Transactions: required vs optional

- **No transaction required**:
  - acquire lease (`PutItem`/`UpdateItem` with a condition expression)
  - refresh lease (`UpdateItem` with `lease_token` check)
  - release lease (best-effort delete or conditional update)
  - read metadata (`GetItem`)
- **Use a transaction (recommended)** when a single regeneration needs to update **multiple items atomically**, e.g.:
  - write new metadata + release lease as one atomic step
  - pointer-swap designs where you write a new version item and then update the “current” pointer

(See `docs/facetheory/isr-transaction-recipes.md` once FT-T2 lands.)

## Runnable model definitions

### Go (struct tags)

```go
package models

import "os"

type FaceTheoryCacheMetadata struct {
	PK string `theorydb:"pk,attr:pk" json:"pk"`
	SK string `theorydb:"sk,attr:sk" json:"sk"`

	S3Key            string `theorydb:"attr:s3_key" json:"s3_key"`
	GeneratedAt      int64  `theorydb:"attr:generated_at" json:"generated_at"`
	RevalidateSeconds int64 `theorydb:"attr:revalidate_seconds" json:"revalidate_seconds"`
	ETag             string `theorydb:"attr:etag,omitempty" json:"etag,omitempty"`

	TTL int64 `theorydb:"ttl,attr:ttl,omitempty" json:"ttl,omitempty"`
}

func (FaceTheoryCacheMetadata) TableName() string {
	return os.Getenv("FACETHEORY_CACHE_TABLE_NAME")
}
```

```go
package models

import "os"

type FaceTheoryCacheLease struct {
	PK string `theorydb:"pk,attr:pk" json:"pk"`
	SK string `theorydb:"sk,attr:sk" json:"sk"`

	LeaseToken     string `theorydb:"attr:lease_token" json:"lease_token"`
	LeaseExpiresAt int64  `theorydb:"attr:lease_expires_at" json:"lease_expires_at"`

	TTL int64 `theorydb:"ttl,attr:ttl,omitempty" json:"ttl,omitempty"`
}

func (FaceTheoryCacheLease) TableName() string {
	return os.Getenv("FACETHEORY_CACHE_TABLE_NAME")
}
```

### TypeScript (`defineModel`)

```ts
import { defineModel } from '@theory-cloud/tabletheory-ts';

export const faceTheoryCacheMetadataModel = defineModel({
  name: 'FaceTheoryCacheMetadata',
  table: { name: process.env.FACETHEORY_CACHE_TABLE_NAME! },
  keys: {
    partition: { attribute: 'pk', type: 'S' },
    sort: { attribute: 'sk', type: 'S' },
  },
  attributes: [
    { attribute: 'pk', type: 'S', roles: ['pk'] },
    { attribute: 'sk', type: 'S', roles: ['sk'] },
    { attribute: 's3_key', type: 'S', required: true },
    { attribute: 'generated_at', type: 'N', required: true },
    { attribute: 'revalidate_seconds', type: 'N', required: true },
    { attribute: 'etag', type: 'S', optional: true },
    { attribute: 'ttl', type: 'N', roles: ['ttl'], optional: true },
  ],
});
```

```ts
import { defineModel } from '@theory-cloud/tabletheory-ts';

export const faceTheoryCacheLeaseModel = defineModel({
  name: 'FaceTheoryCacheLease',
  table: { name: process.env.FACETHEORY_CACHE_TABLE_NAME! },
  keys: {
    partition: { attribute: 'pk', type: 'S' },
    sort: { attribute: 'sk', type: 'S' },
  },
  attributes: [
    { attribute: 'pk', type: 'S', roles: ['pk'] },
    { attribute: 'sk', type: 'S', roles: ['sk'] },
    { attribute: 'lease_token', type: 'S', required: true },
    { attribute: 'lease_expires_at', type: 'N', required: true },
    { attribute: 'ttl', type: 'N', roles: ['ttl'], optional: true },
  ],
});
```

### Python (dataclass + `ModelDefinition`)

```py
from __future__ import annotations

import os
from dataclasses import dataclass

from theorydb_py.model import ModelDefinition, theorydb_field


@dataclass
class FaceTheoryCacheMetadata:
    pk: str = theorydb_field(roles=["pk"], name="pk")
    sk: str = theorydb_field(roles=["sk"], name="sk")

    s3_key: str = theorydb_field(name="s3_key")
    generated_at: int = theorydb_field(name="generated_at")
    revalidate_seconds: int = theorydb_field(name="revalidate_seconds")
    etag: str | None = theorydb_field(name="etag", omitempty=True, default=None)
    ttl: int | None = theorydb_field(roles=["ttl"], name="ttl", omitempty=True, default=None)


FACE_THEORY_CACHE_METADATA = ModelDefinition.from_dataclass(
    FaceTheoryCacheMetadata,
    table_name=os.environ["FACETHEORY_CACHE_TABLE_NAME"],
)
```

```py
from __future__ import annotations

import os
from dataclasses import dataclass

from theorydb_py.model import ModelDefinition, theorydb_field


@dataclass
class FaceTheoryCacheLease:
    pk: str = theorydb_field(roles=["pk"], name="pk")
    sk: str = theorydb_field(roles=["sk"], name="sk")

    lease_token: str = theorydb_field(name="lease_token")
    lease_expires_at: int = theorydb_field(name="lease_expires_at")
    ttl: int | None = theorydb_field(roles=["ttl"], name="ttl", omitempty=True, default=None)


FACE_THEORY_CACHE_LEASE = ModelDefinition.from_dataclass(
    FaceTheoryCacheLease,
    table_name=os.environ["FACETHEORY_CACHE_TABLE_NAME"],
)
```

