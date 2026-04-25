# Release-State Patterns for Python

Use release-state helpers when a Python service needs an authoritative registry row plus immutable event history.
The shared contract is described in the root [release-state safety patterns](../../docs/release-state-patterns.md)
guide; this page names the Python API surface.

## APIs

```python
from theorydb_py import (
    ModelDefinition,
    Table,
    WritePolicy,
    transition_release_state,
    validate_deploy_authority_metadata,
)
```

- Declare model-level mutation rules through `WritePolicy` when building a `ModelDefinition`.
- Use `transition_release_state(...)` to update the actual-state row and append an event row in one DynamoDB
  transaction.
- Use `validate_deploy_authority_metadata(...)` before persisting deploy-authoritative `provenance`/`confidence`
  metadata.

## Guardrails

✅ Correct:

- `WritePolicy(mode="write_once")` for event-history models.
- `WritePolicy(mode="mutable", protected_attributes=[...])` for registry pin fields.
- A single `transition_release_state(...)` call when actual/event rows share the same DynamoDB client/table context.
- Explicit outbox/retry/reconciliation for Lambda alias, CodePipeline, or other external side effects.

❌ Incorrect:

- Mutating event history through generic update/save APIs.
- Using separate DynamoDB calls for the actual transition and event append.
- Treating external side effects as DynamoDB-atomic.
- Persisting low-confidence or conflicting evidence as deploy authority.

See `py/examples/release_state.py` for a cross-runtime-aligned example.
