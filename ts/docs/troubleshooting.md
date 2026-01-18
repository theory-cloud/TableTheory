# Troubleshooting (TypeScript)

This guide maps common problems to verified fixes for the TypeScript SDK.

## Error: `ENOENT: .../package.json` when packaging

**Cause:** Running `npm pack` outside of `ts/` (npm expects a `package.json` in the working directory).

**Solution:** Run packaging from inside `ts/`:

```bash
pushd ts
npm ci
npm run build
npm pack --pack-destination ../release-assets
popd
```

## Error: `CredentialsProviderError` when using DynamoDB Local

**Cause:** AWS SDK v3 still requires credentials even for DynamoDB Local.

**Solution:** Provide dummy credentials:

```ts
const ddb = new DynamoDBClient({
  region: 'us-east-1',
  endpoint: 'http://localhost:8000',
  credentials: { accessKeyId: 'dummy', secretAccessKey: 'dummy' },
});
```

## Error: Encrypted attribute used as a key

**Cause:** Encrypted attributes are not allowed for PK/SK or index keys.

**Solution:** Move secrets to non-key attributes and keep keys plaintext/stable.
