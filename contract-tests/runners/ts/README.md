# TypeScript Contract Runner (stub)

This runner is the TypeScript counterpart of `contract-tests/runners/go`.

Right now it:
- loads DMS + scenario YAML fixtures
- validates the golden cursor fixture decodes correctly

Once `tabletheory-ts` exists, this runner should be extended by implementing `src/driver.ts` and wiring the scenario steps
to real DynamoDB operations.

## Run

```bash
cd contract-tests/runners/ts
npm install
npm run test:fixtures
```

