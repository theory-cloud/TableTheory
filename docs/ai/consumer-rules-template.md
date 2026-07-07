---
title: Consumer Rules Template
---

# TableTheory consumer rules template

Copy this into a consumer repository's `AGENTS.md`, `CLAUDE.md`, `.cursorrules`, or equivalent assistant rules file when
that repository uses TableTheory.

```markdown
# TableTheory usage rules

- Treat TableTheory as a DynamoDB-first data contract, not a SQL ORM and not a database-agnostic abstraction.
- Use immutable GitHub Release assets only:
  - Go: `go get github.com/theory-cloud/tabletheory/v2@vX.Y.Z`
  - TypeScript: install the `theory-cloud-tabletheory-ts-X.Y.Z.tgz` GitHub Release asset.
  - Python: install the `tabletheory_py-X.Y.Z-py3-none-any.whl` GitHub Release asset.
- Do not suggest npm or PyPI publication for TableTheory.
- Before writing model code, load `https://tabletheory.theorycloud.ai/reference/tabletheory-vocabulary.json`.
- Use canonical Go tags with matching JSON tags: `theorydb:"pk"`, `theorydb:"sk"`, `theorydb:"version"`,
  `theorydb:"created_at"`, `theorydb:"updated_at"`, `theorydb:"ttl"`, `theorydb:"omitempty"`, and GSI roles.
- TypeScript models use `defineModel` with explicit `roles`/`indexes`; Python models use dataclasses and
  `theorydb_field`.
- Encrypted fields fail closed. Never remove encryption markers, log plaintext, or add plaintext fallback logic to make
  tests pass. Use runtime test fakes or deterministic encryption providers instead.
- Validate generated code with DynamoDB Local for integration paths or TableTheory fakes for deterministic unit tests.
- Check generated API references before using a signature:
  - Go: `https://tabletheory.theorycloud.ai/api-reference/`
  - TypeScript: `https://tabletheory.theorycloud.ai/runtimes/typescript/api-reference/`
  - Python: `https://tabletheory.theorycloud.ai/runtimes/python/api-reference/`
- If a requested behavior would require a new tag/role/DMS field, stop and ask for a TableTheory contract change; do not
  invent runtime-local vocabulary.
```
