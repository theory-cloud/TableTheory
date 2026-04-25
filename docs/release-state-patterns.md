# Release-State Safety Patterns

<!-- AI Training: TableTheory release-state guardrails are DynamoDB-first, opt-in, and cross-runtime. -->

Release-state records describe what release, artifact, alias, or deployment pin is authoritative for a service.
Those records are safety-critical: a generic helper should not be able to mutate immutable history, loosen protected
fields, or store ambiguous evidence as deploy authority. TableTheory exposes the release-state contract as an
opt-in DynamoDB pattern that works the same way in Go, TypeScript, and Python.

Use this guide when modeling release registries, deployment pin stores, factory import state, or similar records that
need a mutable actual-state row plus immutable event history.

## Contract summary

| Concern | TableTheory pattern |
| --- | --- |
| Actual-state registry row | Mutable model with protected pin fields |
| Event-history row | `write_once` model |
| Generic mutation guard | Model-level `write_policy` metadata |
| Per-call tightening | Additional protected attributes for that operation only |
| Transition + event append | One DynamoDB transaction when rows share a transaction boundary |
| External side effects | Outbox/retry/reconciliation; never represented as DynamoDB-atomic |
| Deploy authority | Deterministic `provenance` + `confidence` metadata |
| Ambiguous evidence | Rejected for deploy authority; store only as immutable visibility/audit events |

The default remains unchanged for existing models: a model without `write_policy` is mutable and has no protected
attributes.

## Actual state vs. event history

Release-state systems should separate **current authority** from **history**:

- **Actual-state row**: one mutable registry row per deployable scope. It answers "what is current?"
- **Event-history row**: append-only facts about how state changed, what was observed, or why a transition was
  attempted. It answers "what happened?"

✅ Correct single-table shape:

```text
PK = RELEASE#service-a, SK = ACTUAL
PK = RELEASE#service-a, SK = EVENT#2026-04-24T19:00:00Z#rel_002
PK = RELEASE#service-a, SK = EVIDENCE#factory#batch-2026-04-24
PK = RELEASE#service-a, SK = OUTBOX#lambda-alias#rel_002
```

❌ Incorrect shape:

```text
PK = RELEASE#service-a, SK = ACTUAL
notes = "operator thought this probably came from manifest A"
```

Free-form notes are not deploy authority. Store structured evidence in event/evidence rows and put only deterministic
authority metadata on actual-state rows.

## Model-level write policy

Use model metadata to make immutability enforceable by generic TableTheory APIs.

### Actual-state row

Actual-state rows are usually mutable, but fields that identify the registry pin should be protected at the model
level.

```yaml
write_policy:
  mode: mutable
  protected_attributes: ["pinnedReleaseId"]
```

Model-level protected fields cannot be loosened by a helper call. A helper may protect additional fields for one
operation, but it cannot make `pinnedReleaseId` mutable for that operation.

### Event-history row

Event-history rows should be write-once.

```yaml
write_policy:
  mode: write_once
```

`write_once` allows initial creation and rejects generic high-level update, save/upsert, and delete flows. That makes
event history append-only by default. Low-level raw DynamoDB usage remains the escape hatch when an operator is doing
explicit migration or repair work outside the high-level TableTheory contract.

## Transaction boundary

When the actual-state row and event-history row are in DynamoDB under the same transaction boundary, perform the state
transition and event append in one DynamoDB transaction:

1. Update the actual-state row.
2. Increment the `version` field.
3. Check the expected version when supplied.
4. Create the event-history row with a not-exists condition.

That gives consumers one observable outcome: both internal rows commit, or neither commits.

✅ Correct:

```text
TransactWriteItems:
  - Update RELEASE#service-a / ACTUAL
  - Put    RELEASE#service-a / EVENT#...
```

❌ Incorrect:

```text
Update ACTUAL
Put EVENT
```

Two separate calls can leave current state without its audit/event record.

## External side effects are not DynamoDB-atomic

External systems such as Lambda aliases, CodePipeline executions, container image promotions, or DNS updates cannot be
made atomic by a DynamoDB transaction. Do not document or implement a transition as if those systems commit inside the
same transaction.

Use one of these patterns:

### Internal transition before side effect

1. Write the intended internal state and an outbox row in one DynamoDB transaction.
2. A worker reads the outbox row and performs the external side effect.
3. The worker appends success/failure event rows.
4. A reconciler retries or repairs incomplete side effects.

### Side effect before authoritative state

1. Perform the external side effect with an idempotency key.
2. Observe the external system state.
3. Write the actual-state row and event append transaction with deterministic evidence.
4. If the transaction fails, reconcile from the idempotency key and observed external state.

Choose the direction based on the real authority boundary for your system. In both directions, use explicit
idempotency keys and reconciliation. Do not hide external side effects behind a helper that appears to be atomic.

## Outbox records

Outbox records should be immutable event-like rows. They represent work to attempt, not deploy authority by
themselves.

Recommended fields:

| Field | Purpose |
| --- | --- |
| `PK` / `SK` | Scope and stable outbox ID |
| `operation` | External side effect to attempt |
| `idempotencyKey` | Stable key reused on retry |
| `requestedState` | Target release/status snapshot |
| `attempts` | Retry visibility |
| `nextAttemptAt` | Backoff scheduling |
| `lastError` | Redacted error summary |
| `createdAt` / `updatedAt` | Lifecycle timestamps |

Never store secrets, KMS key material, AWS credentials, or encrypted plaintext in outbox records.

## Reconciliation

Every release-state system that touches an external side effect needs a reconciler. A reconciler should:

- query pending outbox rows;
- compare actual-state rows with event history;
- observe external systems through their native APIs;
- append immutable reconciliation events;
- retry idempotently when safe;
- reject conflicting evidence rather than lowering deploy authority confidence.

Low confidence is visibility-only. It is useful for dashboards and investigation, but it must not authorize deployment
state.

## Provenance and confidence metadata

Actual-state rows that claim deploy authority should carry deterministic metadata:

```json
{
  "provenance": {
    "mode": "native",
    "system": "release-control-plane",
    "kind": "operator_command",
    "ref": "operator://deploy/service-a/rel_002",
    "observed_at": "2026-04-24T19:00:00Z",
    "recorded_at": "2026-04-24T19:00:01Z",
    "evidence": [
      {
        "kind": "operator_command",
        "source": "release-control-plane",
        "ref": "operator://deploy/service-a/rel_002",
        "observed_at": "2026-04-24T19:00:00Z"
      }
    ]
  },
  "confidence": {
    "level": "high",
    "reasons": ["operator_command_authority"]
  }
}
```

The helper validation accepts deterministic high-confidence evidence and rejects:

- missing `provenance`/`confidence` pairings;
- unsupported free-form provenance keys;
- non-RFC3339 timestamps;
- conflicting evidence signatures;
- unsupported evidence kinds/sources;
- `medium` or `low` confidence as deploy authority;
- confidence reasons that do not match the deterministic evidence mapping.

If import evidence is ambiguous, preserve it as immutable evidence/event rows. Do not write it to the actual-state row
as deploy authority.

## DynamoDB Streams audit/archive

DynamoDB Streams are the right integration point for secondary audit archives, search indexes, or compliance sinks.
Streams are not the primary authority for release-state correctness; the write policy and transaction are. Use Streams
to mirror committed facts after the DynamoDB transaction succeeds.

Recommended stream handling:

- treat stream delivery as at-least-once;
- make archive writes idempotent by event row key;
- redact encrypted/sensitive fields before logging;
- alert on stream lag, but reconcile from DynamoDB state rather than assuming the stream archive is authoritative.

## Release/RC verification checklist

Before promoting a TableTheory release candidate that includes release-state safety support:

1. Verify Go, TypeScript, and Python contract runners execute release-state scenarios 06–09.
2. Run `make rubric`.
3. Run `bash scripts/verify-branch-release-supply-chain.sh`.
4. Run `bash scripts/verify-branch-version-sync.sh`.
5. Confirm release notes classify the feature as additive/opt-in.
6. Validate downstream release-control-plane models against the RC asset.
7. Confirm distribution remains GitHub Releases only. Do not publish to npm or PyPI.

## Runtime docs

- Go examples live under `examples/`.
- TypeScript package docs live under `ts/docs/`.
- Python package docs live under `py/docs/`.
