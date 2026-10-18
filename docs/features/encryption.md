---
title: Encryption
description: KMS-backed field encryption — fail-closed, no fallbacks, identical across all three runtimes.
---

# Encryption

`encrypted` marks a field for KMS-backed encryption. The behavior is **fail-closed** and that property is load-bearing across the entire Theory Cloud stack.

If the KMS key is not configured, encrypted reads return an error. The runtime does **not**:

- silently return plaintext,
- fall back to a different key,
- skip decryption "for development convenience,"
- emit a warning and continue.

Every Theory Cloud consumer that stores sensitive data — Autheory's credentials, theory-mcp-server's identity bindings, AppTheory's PII fields — trusts this behavior absolutely. Softening it is the refusal that comes up most often in security-adjacent scoping conversations, and it remains a refusal.

## How it works

1. The KMS key id (or alias) is bound to the client at construction.
2. On write, fields with the `encrypted` role/flag are run through KMS `Encrypt` and stored as ciphertext blobs.
3. On read, the ciphertext is run through KMS `Decrypt`. Failure to decrypt = the read fails.
4. Encrypted values are **never logged** — the redaction layer suppresses them at logging sites.

## Go

```go
type Credential struct {
    PK     string `theorydb:"pk"        json:"pk"`
    SK     string `theorydb:"sk"        json:"sk"`
    Secret string `theorydb:"encrypted" json:"secret"`
}

// Provide KMS configuration via the standard Config / environment
// surface (see api-reference.md for the full set). The runtime fails
// closed when an encrypted field is touched without configured KMS.
```

## TypeScript

```typescript
const Credential = defineModel({
  name: 'Credential',
  table: { name: 'credentials_contract' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort:      { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK',     type: 'S', roles: ['pk'] },
    { attribute: 'SK',     type: 'S', roles: ['sk'] },
    { attribute: 'secret', type: 'S', encryption: { v: 1 } },
  ],
});
```

## Python

```python
@dataclass(frozen=True)
class Credential:
    pk:     str = theorydb_field(roles=["pk"])
    sk:     str = theorydb_field(roles=["sk"])
    secret: str = theorydb_field(encrypted=True)
```

## Record binding (envelope v2)

Encrypted attributes are stored as an envelope map: `v`, `edk`, `nonce`, `ct`.
Envelope **v2** authenticates the record identity in the AES-GCM additional
data:

```text
theorydb:encrypted:v2|attr=<len>:<attribute>|table=<len>:<table>|pk=<len>:<key>|sk=<len>:<key>
```

* Every variable-length component is prefixed with its UTF-8 byte length.
  Length framing is unambiguous: a table, attribute, or key value containing
  `|` or `:` cannot collide with another record identity.
* `pk` / `sk` use the canonical key encoding `S:<value>`, `N:<value>`, or
  `B:<base64>`; `sk` is present only when the record has a sort key. Key types
  other than `S`, `N`, and `B` fail closed rather than canonicalizing to an
  empty value.
* The same AAD is supplied to KMS as the `EncryptionContext` key
  `theorydb:encryption:aad` on both `GenerateDataKey` and `Decrypt` for v2
  envelopes, so the wrapped data key is bound to the record identity at KMS
  as well as in AES-GCM. v1 envelopes carry no KMS context so existing data
  stays readable.
* A v2 envelope copied to another record or another table fails to decrypt.
  This closes cross-record and cross-model envelope replay.
* **v1 envelopes remain readable.** Existing stored data decrypts exactly as
  before; no migration is required. The write paths only start producing v2
  once a runtime that supports it writes the record again.
* A v2 envelope needs the record key to decrypt. A projected read that omits
  `PK`/`SK` fails closed with `ErrInvalidEncryptedEnvelope`; include the key
  attributes in the projection when reading encrypted fields.

The AAD string and canonical key encoding are a cross-runtime contract shared
by Go, TypeScript, and Python.

## What "fail-closed" actually means

- **No KMS key configured + encrypted field write**: write fails before touching DynamoDB.
- **KMS key revoked / inaccessible + encrypted field read**: read fails, returning a typed encryption error.
- **Decryption fails (wrong key, corrupted ciphertext)**: read fails. The plaintext is never inferred or guessed.
- **A v2 envelope is replayed onto another record or table**: read fails; the additional authenticated data does not match.
- **Cross-region replication**: the consumer must configure the encryption key in the target region; otherwise reads from that region fail closed.

## Local development

For local testing where KMS isn't available, the **caller** must explicitly opt every encrypted field out at the test fixture level — never via a global "disable encryption" flag. The default fail-closed behavior never softens. See [Testing](../testing-guide.md) for the supported per-fixture pattern.

## Anti-patterns

- **"Let's silently fall back to plaintext if KMS is down."** No. This is a proposal to exfiltrate sensitive data and is refused on sight.
- **"Let's add a global env flag to disable encryption in dev."** No. The caller pattern at the test fixture level is the only supported escape hatch.
- **"Let's catch and log the KMS error instead of returning it."** No. Encrypted values must not be in logs; logging the failure context with the value is itself a leak.

## Related

- [Development Guidelines](https://tabletheory.theorycloud.ai/development-guidelines/) — when to tag a field encrypted
- [Testing](../testing-guide.md) — how to write tests that exercise encrypted fields without KMS
- [Integrations · Autheory](../integrations/autheory.md) — the largest consumer of encrypted fields in the stack
- [Contract Scenarios](../reference/contract-scenarios.md) — fail-closed behavior is exercised on every commit
