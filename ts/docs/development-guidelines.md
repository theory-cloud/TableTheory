# Development Guidelines (TypeScript)

This guide covers development and contribution conventions for the TypeScript SDK in `ts/`.

## Toolchain (pinned)

- Node.js: **24**
- TypeScript: see `ts/package.json`
- Linting: ESLint (must be `--max-warnings=0`)
- Formatting: Prettier

## Common Commands

From the repo root:

- Install: `npm --prefix ts ci`
- Build: `npm --prefix ts run build`
- Typecheck: `npm --prefix ts run typecheck`
- Lint: `npm --prefix ts run lint`
- Format check: `npm --prefix ts run format:check`
- Unit tests: `npm --prefix ts run test:unit`
- Integration tests (DynamoDB Local): `npm --prefix ts run test:integration`

## Coding Standards

- Prefer explicit attribute names in model definitions (DMS-friendly).
- Keep public APIs stable and documented in [API Reference](./api-reference.md).
- Do not weaken testkit strictness: unit tests should fail if expected AWS commands were not issued.
- Treat `encrypted` fields as fail-closed: do not allow silent plaintext fallbacks.

## Type-aware lint posture

- `recommended-type-checked` is active for the TypeScript runtime, and `no-floating-promises` is load-bearing.
- A narrow legacy allowlist remains in `ts/eslint.config.js` for high-churn typed-safety cleanup rules, including the
  current `no-unsafe-*` debt around dynamic DynamoDB marshalling and testkit boundaries.
- Do not expand that allowlist as routine cleanup. Tighten it in a dedicated typed-safety pass that can cover affected
  call sites without unrelated public API or runtime behavior churn.
