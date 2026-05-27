---
title: Autheory
description: How Autheory persists identity, sessions, and credentials through TableTheory's encryption primitive.
---

# Autheory

[Autheory](https://github.com/theory-cloud/autheory) is the identity layer for the Theory Cloud stack. Every identity concept — users, clients, tenants, workspaces, memberships, sessions, MFA enrollments, credentials — is persisted through TableTheory.

Autheory is the single largest consumer of TableTheory's encryption primitive, and the canonical test of its fail-closed posture.

## What Autheory persists through TableTheory

| Autheory surface           | TableTheory primitives                                   |
|----------------------------|----------------------------------------------------------|
| Users                      | Single-table model, optimistic locking                   |
| Tenants & workspaces       | Composite keys, tenant-partitioned                       |
| Memberships                | Inverse-GSI for "who belongs where"                      |
| Sessions                   | TTL + optimistic locking                                 |
| MFA enrollments            | **Encrypted fields**, optimistic locking                 |
| Credentials                | **Encrypted fields**, fail-closed reads                  |
| Refresh tokens             | TTL + encrypted bound-secret                             |
| Audit log                  | Append-only writes, time-ordered sort keys               |

## The encryption test

Autheory is the reason TableTheory's fail-closed encryption posture cannot soften. If a credential read silently returned plaintext when KMS was unavailable, every Autheory consumer downstream — meaning every Theory Cloud product — would silently lose secrecy. The refusal to weaken fail-closed is upheld for Autheory in particular.

## Cross-runtime importance

Autheory itself is Go, but tokens it issues are validated by services in all three runtimes (Go, TypeScript, Python). The persistence layer's cross-runtime parity is therefore directly on Autheory's hot path — a bug where Python reads a session record differently than Go is a real-world authentication failure.

## Why this matters for TableTheory stewardship

Autheory's correctness depends on three TableTheory invariants holding simultaneously:

1. **Encryption fail-closed.** Discussed above.
2. **Optimistic locking determinism.** Session refresh races must produce one winner.
3. **TTL honor on read.** Expired sessions must never return data on `Get`.

Any of these regressing breaks Autheory. The [contract scenarios](../../reference/contract-scenarios/) cover all three on every commit.

## Related

- [Features · Encryption](../../features/encryption/) — fail-closed, no fallbacks
- [Features · Optimistic Locking](../../features/optimistic-locking/) — session refresh semantics
- [Features · TTL](../../features/ttl/) — session expiration semantics
- [Autheory repository](https://github.com/theory-cloud/autheory)
