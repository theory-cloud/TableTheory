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

## Error: transaction canceled (`TransactionCanceledException`)

**Cause:** a DynamoDB transaction is atomic. When any operation's condition fails, nothing is committed and DynamoDB
returns a `TransactionCanceledException` with one cancellation reason per operation, in order (`"None"` for the
operations that did not fail). TableTheory classifies that exception by the FIRST genuine failure — the first reason
whose code is neither missing nor `"None"`:

| DynamoDB reason code                                                                                                        | `TheorydbError.code`     |
| --------------------------------------------------------------------------------------------------------------------------- | ------------------------ |
| `ConditionalCheckFailed`                                                                                                    | `ErrConditionFailed`     |
| `TransactionConflict`                                                                                                       | `ErrTransactionConflict` |
| `ThrottlingException`, `ProvisionedThroughputExceeded`, `RequestLimitExceeded`, `InternalServerError`, `ServiceUnavailable` | `ErrThrottled`           |
| anything else, no genuine failure, or an empty reason list                                                                  | `ErrTransactionFailed`   |

An unknown or missing code is never reported as a condition failure, and an ambiguous transport error (a timeout, a
connection reset) is returned unchanged rather than being reclassified.

**Fix:** switch on the specific code and read the ordered reasons and original exception:

```ts
try {
  await client.transactWrite([...]);
} catch (err) {
  if (hasTheorydbErrorCode(err, 'ErrConditionFailed')) {
    // A conditional write lost; nothing was committed.
  }
  if (err instanceof TheorydbError) {
    for (const reason of err.cancellationReasons) {
      // index / code / message only — never item images.
      console.log(reason.index, reason.code, reason.message);
    }
    const original = err.cause; // the SDK TransactionCanceledException
  }
}
```

`cancellationReasons` is ordered and position-preserving. **The typed cancellation fields never carry item
images:** each reason holds only `index`, `code` and `message`, and `err.code`, `err.message` and `String(err)`
never include item data or attribute values.

The original SDK `TransactionCanceledException` is retained as `err.cause`. When the request returned item images
(for example with `ReturnValuesOnConditionCheckFailure`) that exception still holds them, and `cause` is a plain
enumerable property — so `JSON.stringify(err)`, `console.log(err)` (`util.inspect`) and any logger that walks the
error object serializes them too. Log the typed fields instead of the whole error:

```ts
if (err instanceof TheorydbError) {
  // Safe: reasons carry only index / code / message — never item images.
  logger.warn(
    { code: err.code, reasons: err.cancellationReasons },
    'transaction canceled',
  );
}
```

Do not `JSON.stringify(err)` or log the whole error object when item images are sensitive.

`safeErrorFields(err)` returns the typed surface only, and is the projection to log
or serialize:

```ts
import { safeErrorFields } from '@theory-cloud/tabletheory-ts';

const fields = safeErrorFields(err);
// { code, codes, message, cancellationReasons: [{ index, code, message }] }
logger.warn({ error: fields }, 'transaction canceled');
```

It never includes `cause`, so it cannot reach an item image, and it returns
`undefined` when the value is not a `TheorydbError`.

To strip the item images from the retained cause itself, construct the client with
`redactItemImages`:

```ts
const client = new TheorydbClient(ddb, { redactItemImages: true });
```

It is **off by default**, so `err.cause` is the same object it always was. With it
enabled, `cause` becomes a copy whose `CancellationReasons[].Item` (and a
condition failure's `Item`) are removed, and the original exception is never
mutated. Until you enable it, dumping the whole error with the default settings can
still reach the item images; changing that default would change `cause` for every
consumer, so it belongs to a future major release rather than a change made here.

**Coverage boundary.** The option plumbs through `TheorydbClient`: every error a
client maps is redacted. Two paths map errors directly and cannot inherit it — the
three `mapDynamoError` call sites in `lease.ts` and `update-builder`'s `execute()`
— and there is no standalone TypeScript wrapper equivalent to Python's
`redacting_client`. That is safe today because no TypeScript or Python code sets
`ReturnValuesOnConditionCheckFailure`, so those paths cannot carry item images.
Revisit this boundary if `ALL_OLD` (`ReturnValuesOnConditionCheckFailure`) support
is ever added.

**Compatibility note:** TypeScript previously reported EVERY transaction cancellation as `ErrConditionFailed`.
Throttling and conflict cancellations now report `ErrThrottled` / `ErrTransactionConflict`, and a cancellation with no
genuine failure reports `ErrTransactionFailed`. Code that switched on `ErrConditionFailed` should switch on the
specific code, or on `hasTheorydbErrorCode`.

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
