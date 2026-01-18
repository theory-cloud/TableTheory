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
