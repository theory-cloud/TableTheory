# TableTheory Model Contract v0.1 derived-key sidecar

`tabletheory_model_contract` v0.1 is an additive sidecar for DMS v0.1. It lets a consumer declare derived DynamoDB key strings once as ordered, language-neutral templates, then evaluate the same bytes in Go and TypeScript.

This sidecar does **not** change DMS v0.1 core semantics. DMS still owns model shape, tags, attributes, keys, indexes, write policy, and validation. The sidecar owns only model metadata discovery plus derived-key templates and golden fixtures.

## Boundary

TableTheory owns:

- the sidecar schema;
- the template grammar;
- Go and TypeScript evaluators;
- the TypeScript helper generator;
- cross-runtime fixture verification.

Consumers own:

- which models expose derived keys;
- the segment formulas supplied in their sidecar file;
- product validation such as route-token syntax, enum values, tenant/agent authorization, or management-mode semantics.

A TableTheory evaluator must never silently normalize beyond the declared segment transforms. If Go and TypeScript outputs diverge on a fixture, the fixture fails.

## Document shape

```yaml
tabletheory_model_contract_version: "0.1"
namespace: "example.product"
dms_version: "0.1"
models:
  - name: "ExampleModel"
    dms_model: "ExampleModel"
    derived_keys: ["ExampleKey"]
derived_keys:
  - name: "ExampleKey"
    helper: "exampleKey"
    join: "|"
    inputs:
      - { name: "tenant_id", ts_name: "tenantId", type: "string" }
      - { name: "partner_id", ts_name: "partnerId", type: "string", optional: true }
    segments:
      - { prefix: "tenant=", value: { input: "tenant_id" }, transforms: ["trim"] }
      - { prefix: "partner=", value: { input: "partner_id" }, transforms: ["trim", "wildcard_empty"] }
    fixtures:
      - name: "default_partner"
        input: { tenant_id: " tenant-a ", partner_id: "" }
        expect: "tenant=tenant-a|partner=*"
```

## Segment evaluation

For each segment, evaluators perform the following steps in order:

1. Resolve exactly one source: `value.input`, `value.literal`, or segment-level `literal`.
2. Convert scalar input values to strings (`string`, finite `number`, `boolean`; `null`/missing optional inputs become empty).
3. Apply transforms in the order declared.
4. If the result is empty and `default` is present, use `default`.
5. Apply `omit_when` rules.
6. Append `prefix + value + suffix` to the ordered segment list if not omitted.
7. Join emitted segments with `join` exactly.

Supported transforms in v0.1:

- `trim` — remove leading and trailing Unicode whitespace.
- `wildcard_empty` — map the empty string to `*`. It does not trim by itself; declare `trim` before it when whitespace should become wildcard-eligible.

Supported omission rules:

- `optional: true` — missing or empty source without a default omits the segment.
- `omit_when.empty: true` — omit after transforms/defaults when the value is exactly empty.
- `omit_when.default: true` — omit when the value equals the segment default.
- `omit_when.values: [...]` — omit when the value exactly matches one of the listed strings.

## Generator

The Go command emits a TypeScript module with typed helper functions and an embedded contract:

```bash
go run ./cmd/tabletheory-contract generate-ts \
  --contract contract-tests/key-contracts/v0.1/theorymcp-derived-keys.yml \
  --out /tmp/theorymcp-derived-keys.ts
```

Generated helpers import the runtime evaluator from `@theory-cloud/tabletheory-ts` by default. Use `--runtime-import` only when generating inside a monorepo test harness that needs a different import path.

## Current fixtures

`contract-tests/key-contracts/v0.1/theorymcp-derived-keys.yml` is a product fixture file supplied as a TableTheory contract example. It mirrors representative TheoryMCP Go key helper formulas without baking those product semantics into the framework:

- `WildcardScope`
- `CanonicalPolicyKey`
- `CanonicalBindingKey`
- `InterfaceScopeKey`
- `SkillScopeKey`
- email/GitHub binding scope and sort keys
- GitHub installation/repository lookup keys
- `ImportSessionScopeKey`

Future consumer fixture files can be dropped under `contract-tests/key-contracts/v0.1/` and exercised by the same Go/TypeScript evaluators without code changes.
