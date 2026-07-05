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

## Error: `ErrTableNotFound` or DynamoDB `ResourceNotFoundException`

**Cause:** the table name in `defineModel({ table: { name } })` does not exist at the configured endpoint/region, or the
client is pointed at AWS when you expected DynamoDB Local.

**Fix:** verify the model table name and endpoint, then create the table through your infrastructure or test setup.

```ts
const ddb = new DynamoDBClient({
  region: 'us-east-1',
  endpoint: process.env.DYNAMODB_ENDPOINT ?? 'http://localhost:8000',
  credentials: { accessKeyId: 'dummy', secretAccessKey: 'dummy' },
});
```

## Error: `ErrMissingPrimaryKey` or empty key attributes

**Cause:** the item passed to `create`, `get`, `update`, or `delete` is missing the declared PK/SK attributes. DynamoDB
keys cannot be empty strings.

**Fix:** construct keys in one place and keep the attribute names identical to the model schema.

```ts
const key = { PK: 'USER#42', SK: 'PROFILE' };
await db.get('User', key);
```

## Error: GSI query returns no items or rejects consistent reads

**Cause:** querying a GSI requires `.usingIndex(name)`, and DynamoDB does not support strongly consistent reads on GSIs.

**Fix:** specify the index and remove `consistentRead(true)` for GSI reads.

```ts
await db
  .query('User')
  .usingIndex('gsi_email')
  .partitionKey('EMAIL#a@example.com')
  .page();
```

## Error: `ErrEncryptionNotConfigured`

**Cause:** a model contains `encryption: { v: 1 }`, but the `TheorydbClient` has no `EncryptionProvider`.

**Fix:** configure `AwsKmsEncryptionProvider` in production, or `createDeterministicEncryptionProvider` in tests. Do not
remove the encryption marker to make a local test pass; encrypted fields fail closed by design.

## Error: `ErrEncryptedFieldNotQueryable`

**Cause:** encrypted attributes cannot appear in key conditions, filters, or update conditions because DynamoDB cannot
compare encrypted envelopes as plaintext.

**Fix:** query by stable plaintext keys (for example `PK`, `SK`, `GSI1PK`, `GSI1SK`) and store secrets in non-key
encrypted attributes.

## Pagination surprise: `all()` loads too much data

**Cause:** `all()` follows every cursor and materializes the full result set.

**Fix:** use `page()`, `pages()`, or `items()` for large partitions and stop iteration when the caller has enough data.

```ts
for await (const item of db
  .query('User')
  .partitionKey('TENANT#1')
  .limit(100)
  .items()) {
  if (done(item)) break;
}
```

## Testing surprise: mock assertions are flaky around timestamps

**Cause:** lifecycle timestamps are generated at write time.

**Fix:** inject a deterministic clock with `now` and use the testkit fake for DynamoDB calls.

```ts
const db = new TheorydbClient(mock.client, {
  now: () => '2026-01-16T00:00:00.000000000Z',
}).register(User);
```
