---
title: Struct Definition Guide
---

# Struct Definition Guide

This guide documents the **canonical** way to define TableTheory models using Go struct tags.

If you are working in a security-critical domain (PHI/PII/CHD), treat model definitions as part of your attack surface:
incorrect tags can lead to data integrity issues, confusing access patterns, or unexpected attribute writes.

## Minimal model (partition key + sort key)

Every TableTheory model must define:

- a partition key: `theorydb:"pk"`
- a sort key: `theorydb:"sk"`

Recommended: include matching `json:"..."` tags for stable external naming.

```go
type User struct {
	ID    string `theorydb:"pk" json:"id"`
	Email string `theorydb:"sk" json:"email"`

	Name string `json:"name"`
}
```

## Table names

If a model does not define `TableName() string`, TableTheory derives a table name with a simple pluralization rule:

- names ending in `s` get `es`
- names ending in `y` become `ies`
- all other names get `s`

Production models should steer table names explicitly instead of relying on pluralization:

```go
func (User) TableName() string { return "users_contract" }
```

## Attribute naming

By default, TableTheory uses your field name (or the configured naming convention) as the DynamoDB attribute name.

TableTheory supports four attribute naming conventions:

- `camelCase` (default)
- `snake_case` (opt-in)
- `pascalCase` (opt-in; useful for legacy tables that use `ID`, `SK`, `GSI1PK`, etc)
- `dynamorm` (opt-in; preserves legacy DynamORM semantics where primary keys are `PK`/`SK` and other fields are camelCase)

To select a convention for a model, add a marker field (commonly a blank identifier) with a `naming:` tag:

```go
type LegacyUser struct {
	_ struct{} `theorydb:"naming:pascalCase"`

	ID string `theorydb:"pk"`
	SK string `theorydb:"sk"`
}

type LegacyDynamORMUser struct {
	_ struct{} `theorydb:"naming:dynamorm"`

	UserID    string `theorydb:"pk"`
	Entity    string `theorydb:"sk"`
	FirstName string
}
```

To override the DynamoDB attribute name explicitly, use:

- `theorydb:"attr:<attributeName>"`

```go
type User struct {
	ID   string `theorydb:"pk" json:"id"`
	Name string `theorydb:"attr:full_name" json:"full_name"`
}
```

## Secondary indexes

### Global secondary indexes (GSI)

Use `index:<indexName>,pk` and `index:<indexName>,sk` to map a field to a GSI key.

```go
type User struct {
	ID    string `theorydb:"pk" json:"id"`
	Email string `theorydb:"sk" json:"email"`

	GSI1PK string `theorydb:"index:user-email-index,pk" json:"gsi1pk"`
	GSI1SK string `theorydb:"index:user-email-index,sk" json:"gsi1sk"`
}
```

### Local secondary indexes (LSI)

Use `lsi:<indexName>` to map a field as an LSI sort key (the table partition key is reused).

```go
type Item struct {
	PK     string `theorydb:"pk" json:"pk"`
	SK     string `theorydb:"sk" json:"sk"`
	Status string `theorydb:"lsi:status-index" json:"status"`
}
```

Legacy `index:lsi-*` / `index:lsi_*` prefix inference is still accepted for compatibility, but registration records a
metadata warning. Prefer `theorydb:"lsi:<indexName>"` for new or edited models so the index type is explicit.

## Field-level encryption (`encrypted`)

Use `theorydb:"encrypted"` to store an attribute encrypted at rest using AWS KMS envelope encryption (AES-256-GCM + KMS data key).

Rules:

- `session.Config.KMSKeyARN` is required for any model with encrypted fields (TableTheory fails closed if it is empty).
- Encrypted fields cannot be used as `pk`, `sk`, or any GSI/LSI key.
- Encrypted fields are not queryable/filterable (ciphertext is non-deterministic). Attempts are rejected with `errors.ErrEncryptedFieldNotQueryable` (from `github.com/theory-cloud/tabletheory/pkg/errors`). If you need lookups, index a separate deterministic value (e.g., a hash).

```go
type Customer struct {
	ID string `theorydb:"pk" json:"id"`

	EmailHash string `theorydb:"index:gsi-email,pk" json:"email_hash"`
	Email     string `theorydb:"encrypted" json:"email"`
}
```

```go
db, err := tabletheory.New(session.Config{
	Region:    "us-east-1",
	KMSKeyARN: os.Getenv("KMS_KEY_ARN"),
})
```

```go
c := &Customer{
	ID:        "cust_1",
	EmailHash: HashEmail("a@example.com"), // application-defined deterministic hash
	Email:     "a@example.com",
}

if err := db.Model(c).Create(); err != nil {
	return err
}

var out Customer
if err := db.Model(&Customer{}).Where("ID", "=", c.ID).First(&out); err != nil {
	return err
}
// out.Email is decrypted.
```

## Optional fields and sets

### Omitting empty values

Use `omitempty` to omit empty values from marshaling.

```go
type User struct {
	ID       string  `theorydb:"pk" json:"id"`
	Nickname *string `theorydb:"omitempty" json:"nickname,omitempty"`
}
```

### String sets

Use `set` to marshal a slice as a DynamoDB set.

```go
type User struct {
	ID   string   `theorydb:"pk" json:"id"`
	Tags []string `theorydb:"set" json:"tags"`
}
```

## Lifecycle fields

These tags are treated specially by TableTheory:

- `created_at`
- `updated_at`
- `version` (optimistic concurrency)
- `ttl` (expiration)

```go
type Record struct {
	ID string `theorydb:"pk" json:"id"`

	CreatedAt time.Time `theorydb:"created_at" json:"created_at"`
	UpdatedAt time.Time `theorydb:"updated_at,omitempty" json:"updated_at,omitempty"`
	Version   int64     `theorydb:"version" json:"version"`
	TTL       int64     `theorydb:"ttl" json:"ttl"`
}
```

## Ignoring fields

Use `theorydb:"-"` to ignore a field entirely.

```go
type User struct {
	ID string `theorydb:"pk" json:"id"`

	CacheKey string `theorydb:"-" json:"-"`
}
```

## Next references

- [Documentation Index](./README.md)
- `docs/core-patterns.md` (canonical usage patterns)
- `docs/api-reference.md` (full API surface)
- Repo-local coding standards live in `docs/development-guidelines.md` and are intentionally excluded from the
  TheoryCloud user-facing surface.
