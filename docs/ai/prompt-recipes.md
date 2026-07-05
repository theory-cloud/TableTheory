---
title: Prompt Recipes
---

# TableTheory prompt recipes

Use these prompts with `llms.txt`, `llms-full.txt`, and the vocabulary JSON when asking an assistant to generate or
review TableTheory code.

## Generate a Go model and CRUD proof

```text
Use TableTheory. First read https://tabletheory.theorycloud.ai/llms.txt and
https://tabletheory.theorycloud.ai/reference/tabletheory-vocabulary.json. Generate a Go model for <access pattern> with
canonical theorydb and json tags, a DynamoDB Local CRUD proof, and tests. Do not use SQL ORM patterns. Do not add new
TableTheory tags. Preserve fail-closed encryption if any field is sensitive.
```

## Generate a TypeScript model

```text
Use @theory-cloud/tabletheory-ts from GitHub Releases. Read the TableTheory vocabulary JSON first. Generate a
`defineModel` schema for <access pattern>, including PK/SK and any GSI in `indexes`. Use `TheorydbClient` APIs from the
generated TypeScript API reference. Include DynamoDB Local credentials for local proof and do not publish to npm.
```

## Generate a Python model

```text
Use tabletheory_py from the TableTheory GitHub Release wheel. Read the vocabulary JSON first. Generate a frozen dataclass
model using `theorydb_field`, `ModelDefinition.from_dataclass`, and `Table`. Include a DynamoDB Local proof or strict fake
unit test. Do not weaken encrypted fields; use `FakeKmsClient` for encrypted tests.
```

## Review generated code for drift

```text
Review this TableTheory change for contract drift. Check that Go, TypeScript, and Python vocabulary is canonical; that
PK/SK/GSI keys are explicit plaintext access-pattern keys; that encrypted fields fail closed; that installation uses
GitHub Releases only; and that any API signatures match the generated API reference. List blockers before suggesting
patches.
```
