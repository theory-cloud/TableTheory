---
title: Generative Coding Artifacts
---

# Generative coding artifacts

TableTheory's generative-coding claim is backed by downloadable artifacts that assistants can load before writing model,
query, or migration code.

| Artifact | Purpose |
| --- | --- |
| [`llms.txt`](../llms.txt) | Small context index for AI tools. |
| [`llms-full.txt`](../llms-full.txt) | Full context pack with invariants and runtime snippets. |
| [`tabletheory-vocabulary.json`](../reference/tabletheory-vocabulary.json) | Machine-readable tags, roles, DMS types, runtime mappings, and invariants. |
| [Consumer rules template](./consumer-rules-template.md) | Copy-in rules for consumer repos using TableTheory. |
| [Prompt recipes](./prompt-recipes.md) | Prompts for generation and drift review. |

Use these with the generated API references so assistants do not invent signatures or runtime-local vocabulary.
