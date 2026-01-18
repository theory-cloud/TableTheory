# TableTheory: `theorydb:"encrypted"` Tag Roadmap (KMS Key ARN only)

Goal: implement **real, enforced field-level encryption semantics** for `theorydb:"encrypted"` in a way that is safe-by-default for high-risk domains (PHI/PII/CHD-like data) and does **not** provide a “metadata-only” false sense of security.

Constraint: encryption must use a **provided AWS KMS Key ARN**. Creating/rotating/managing KMS keys, policies, aliases, and grants is **out of scope** for TableTheory.

## Current state (implemented)

- The `encrypted` tag is **parsed and enforced** across supported read and write surfaces.
- Fields tagged `theorydb:"encrypted"` are encrypted on write and decrypted on read using envelope encryption.
- If any model contains encrypted fields and `session.Config.KMSKeyARN` is empty, TableTheory **fails closed** (no silent plaintext writes).
- Encrypted fields are rejected for PK/SK and GSI/LSI keys, and are not queryable/filterable (returns `errors.ErrEncryptedFieldNotQueryable`).

## Desired semantics (what the tag must mean)

When a struct field has `theorydb:"encrypted"`:

- TableTheory **encrypts the field value before writing** to DynamoDB.
- TableTheory **decrypts the field value after reading** from DynamoDB.
- If encryption is not configured (missing key ARN), TableTheory **fails closed** (errors early; does not silently write plaintext).

## Scope / constraints

- Encrypted fields **must not** be used as:
  - partition key (`pk`) / sort key (`sk`)
  - GSI/LSI keys
  - query/filter operands (non-deterministic ciphertext makes these semantics misleading)
- `omitempty` still applies (an omitted encrypted field is not written).
- `binary` and `json` modifiers are **not** a substitute for encryption; interaction rules must be explicit (see milestones).

## Cryptographic approach (implementation recommendation)

Use **envelope encryption**:

- KMS: `GenerateDataKey` with `KeyId=<provided ARN>` + `KeySpec=AES_256`
- KMS: `Decrypt` with `CiphertextBlob=<EDK>` + `KeyId=<provided ARN>`
- Local: encrypt plaintext bytes with **AES-256-GCM** using a fresh random 12-byte nonce per value
- Store: ciphertext + nonce + encrypted data key (EDK) + a version marker

Suggested stored representation for an encrypted attribute value:

- DynamoDB `M` (map) with stable keys:
  - `v`: number (format version, starting at `1`)
  - `edk`: binary (encrypted data key from KMS)
  - `nonce`: binary (AES-GCM nonce)
  - `ct`: binary (ciphertext)

Security binding (recommended):
- Use AES-GCM AAD derived from the **DynamoDB attribute name** (e.g., `theorydb:encrypted:v1|attr=<dbAttr>`), so swapping ciphertext between attributes fails to decrypt.

## Configuration (fail closed)

Add a single required configuration input:

- `KMSKeyARN` (string) — the KMS key ARN used for `GenerateDataKey`.

Rules:
- If any model contains an encrypted field and `KMSKeyARN` is empty → return an error (preferably at model registration / query compilation time).
- If a ciphertext envelope is encountered but cannot be decrypted → return a typed error (no silent NULL/zero-value substitution).

## Verification & evidence (how we get back to green)

Planned rubric gate:

- `SEC-8`: `bash scripts/verify-encrypted-tag-implemented.sh`

Minimum acceptance evidence for “implemented”:
- Unit tests cover:
  - fail-closed behavior when encrypted fields exist but no key ARN is configured
  - successful round-trip encrypt→store→load→decrypt for representative field types
  - refusal to place `encrypted` on PK/SK/index fields
  - update paths (UpdateBuilder, Update(fields...), transactions, batch operations) handle encryption consistently
- Integration tests (optional but preferred): validate real KMS integration behind an opt-in env gate (kept out of default CI if credentials are unavailable).

## Milestones (implementation plan)

### ENC-0 — Make the gap visible (done when verifiable)

**Goal:** prevent “metadata-only encryption” from passing silently.

**Acceptance criteria**
- `scripts/verify-encrypted-tag-implemented.sh` exists and fails with actionable guidance until ENC-2/ENC-3 are complete.
- Main roadmap includes an “Encryption tag” milestone that maps to `SEC-8`.

---

### ENC-1 — Add configuration plumbing + fail-closed gating

**Goal:** encryption is opt-in via key ARN, but the presence of the tag forces correctness.

**Acceptance criteria**
- Public configuration supports providing a KMS key ARN (no KMS resource management).
- Any attempt to use a model with encrypted fields without a configured key ARN returns an error before writing.
- Unit tests cover the fail-closed behavior.

---

### ENC-2 — Write-time encryption (all write surfaces)

**Goal:** every code path that writes attribute values honors `encrypted`.

**Acceptance criteria**
- Put/Create/Upsert paths encrypt encrypted fields.
- Update paths encrypt encrypted fields (including UpdateBuilder).
- Transaction/batch write paths encrypt encrypted fields.
- Encrypted fields are rejected when used as keys (PK/SK/index keys).

---

### ENC-3 — Read-time decryption (all read surfaces)

**Goal:** every code path that reads attribute values can decode encrypted envelopes safely.

**Acceptance criteria**
- Get/Query/Scan/BatchGet paths decrypt encrypted fields.
- Unknown/invalid envelopes fail with typed errors (no panics).
- Decryption errors do not leak plaintext in error strings.

---

### ENC-4 — Documentation + examples (safe usage)

**Goal:** consumers can use the feature safely without guessing.

**Acceptance criteria**
- Docs clearly state constraints (not queryable, not for keys/indexes).
- Examples show configuration with a key ARN and demonstrate a round-trip.
- Threat model note about `encrypted` being metadata-only is updated once semantics exist.

## Helpful commands

```bash
# Rubric surface (expected red until implemented)
make rubric

# Focused check
bash scripts/verify-encrypted-tag-implemented.sh
```
