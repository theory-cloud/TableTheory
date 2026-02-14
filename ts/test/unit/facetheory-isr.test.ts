import assert from 'node:assert/strict';

import {
  ConditionalCheckFailedException,
  DeleteItemCommand,
  GetItemCommand,
  PutItemCommand,
  TransactWriteItemsCommand,
  TransactionCanceledException,
  type DynamoDBClient,
} from '@aws-sdk/client-dynamodb';

import { TheorydbError } from '../../src/errors.js';
import { FaceTheoryIsrMetaStore } from '../../src/facetheory-isr.js';
import { createMockDynamoDBClient } from '../../src/testkit/index.js';

{
  const mock = createMockDynamoDBClient();
  mock.when(GetItemCommand, async () => ({ $metadata: {} }));

  const store = new FaceTheoryIsrMetaStore({
    ddb: mock.client as unknown as DynamoDBClient,
    tableName: 'tbl',
  });

  const got = await store.get({ cacheKey: 'a' });
  assert.equal(got, null);
}

{
  const mock = createMockDynamoDBClient();
  mock.when(GetItemCommand, async () => ({
    $metadata: {},
    Item: {
      pk: { S: 'CACHE#a' },
      sk: { S: 'META' },
      s3_key: { S: 's3://bucket/key.html' },
      generated_at: { N: '1000' },
      revalidate_seconds: { N: '60' },
      etag: { S: '"abc"' },
    },
  }));

  const store = new FaceTheoryIsrMetaStore({
    ddb: mock.client as unknown as DynamoDBClient,
    tableName: 'tbl',
  });

  const got = await store.get({ cacheKey: 'a' });
  assert.deepEqual(got, {
    htmlPointer: 's3://bucket/key.html',
    generatedAtMs: 1_000_000,
    revalidateSeconds: 60,
    etag: '"abc"',
  });

  const cmd = mock.calls[0];
  assert.ok(cmd instanceof GetItemCommand);
  assert.equal(cmd.input.TableName, 'tbl');
  assert.equal(cmd.input.ConsistentRead, true);
  assert.equal(cmd.input.Key?.pk?.S, 'CACHE#a');
  assert.equal(cmd.input.Key?.sk?.S, 'META');
}

{
  const mock = createMockDynamoDBClient();
  mock.when(PutItemCommand, async () => ({ $metadata: {} }));

  const store = new FaceTheoryIsrMetaStore({
    ddb: mock.client as unknown as DynamoDBClient,
    tableName: 'tbl',
    leaseToken: () => 'tok',
    leaseTtlBufferSeconds: 0,
  });

  const lease = await store.tryAcquireLease({
    cacheKey: 'a',
    nowMs: 1_000_000,
    leaseDurationMs: 30_000,
  });
  assert.deepEqual(lease, { leaseToken: 'tok', leaseExpiresAtMs: 1_030_000 });

  const cmd = mock.calls[0];
  assert.ok(cmd instanceof PutItemCommand);
  assert.equal(cmd.input.TableName, 'tbl');
  assert.equal(cmd.input.Item?.pk?.S, 'CACHE#a');
  assert.equal(cmd.input.Item?.sk?.S, 'LOCK');
  assert.equal(cmd.input.Item?.lease_token?.S, 'tok');
  assert.equal(cmd.input.Item?.lease_expires_at?.N, '1030');
  assert.equal(cmd.input.Item?.ttl, undefined);
}

{
  const mock = createMockDynamoDBClient();
  mock.when(PutItemCommand, async () => {
    throw new ConditionalCheckFailedException({ $metadata: {}, message: 'no' });
  });

  const store = new FaceTheoryIsrMetaStore({
    ddb: mock.client as unknown as DynamoDBClient,
    tableName: 'tbl',
    leaseToken: () => 'tok',
    leaseTtlBufferSeconds: 0,
  });

  const lease = await store.tryAcquireLease({
    cacheKey: 'a',
    nowMs: 1_000_000,
    leaseDurationMs: 30_000,
  });
  assert.equal(lease, null);
}

{
  const mock = createMockDynamoDBClient();
  mock.when(TransactWriteItemsCommand, async () => ({ $metadata: {} }));

  const store = new FaceTheoryIsrMetaStore({
    ddb: mock.client as unknown as DynamoDBClient,
    tableName: 'tbl',
  });

  await store.commitGeneration({
    cacheKey: 'a',
    leaseToken: 'tok',
    nowMs: 1_000_000,
    htmlPointer: 's3://bucket/key.html',
    generatedAtMs: 1_000_000,
    revalidateSeconds: 60,
    etag: '"abc"',
  });

  const cmd = mock.calls[0];
  assert.ok(cmd instanceof TransactWriteItemsCommand);
  assert.equal(cmd.input.TransactItems?.length, 2);

  const [put, del] = cmd.input.TransactItems ?? [];

  assert.equal(put?.Put?.TableName, 'tbl');
  assert.equal(put?.Put?.Item?.pk?.S, 'CACHE#a');
  assert.equal(put?.Put?.Item?.sk?.S, 'META');
  assert.equal(put?.Put?.Item?.s3_key?.S, 's3://bucket/key.html');
  assert.equal(put?.Put?.Item?.generated_at?.N, '1000');
  assert.equal(put?.Put?.Item?.revalidate_seconds?.N, '60');
  assert.equal(put?.Put?.Item?.etag?.S, '"abc"');
  assert.equal(put?.Put?.Item?.ttl, undefined);

  assert.equal(del?.Delete?.TableName, 'tbl');
  assert.equal(del?.Delete?.Key?.pk?.S, 'CACHE#a');
  assert.equal(del?.Delete?.Key?.sk?.S, 'LOCK');
  assert.equal(del?.Delete?.ConditionExpression, '#tok = :tok AND #exp > :now');
  assert.equal(del?.Delete?.ExpressionAttributeNames?.['#tok'], 'lease_token');
  assert.equal(
    del?.Delete?.ExpressionAttributeNames?.['#exp'],
    'lease_expires_at',
  );
  assert.equal(del?.Delete?.ExpressionAttributeValues?.[':tok']?.S, 'tok');
  assert.equal(del?.Delete?.ExpressionAttributeValues?.[':now']?.N, '1000');
}

{
  const mock = createMockDynamoDBClient();
  mock.when(TransactWriteItemsCommand, async () => {
    throw new TransactionCanceledException({ $metadata: {}, message: 'no' });
  });

  const store = new FaceTheoryIsrMetaStore({
    ddb: mock.client as unknown as DynamoDBClient,
    tableName: 'tbl',
  });

  await assert.rejects(
    () =>
      store.commitGeneration({
        cacheKey: 'a',
        leaseToken: 'tok',
        nowMs: 1_000_000,
        htmlPointer: 's3://bucket/key.html',
        generatedAtMs: 1_000_000,
        revalidateSeconds: 60,
      }),
    (e) => e instanceof TheorydbError && e.code === 'ErrLeaseNotOwned',
  );
}

{
  const mock = createMockDynamoDBClient();
  mock.when(DeleteItemCommand, async () => ({ $metadata: {} }));

  const store = new FaceTheoryIsrMetaStore({
    ddb: mock.client as unknown as DynamoDBClient,
    tableName: 'tbl',
    leaseTtlBufferSeconds: 0,
  });

  await store.releaseLease({ cacheKey: 'a', leaseToken: 'tok' });
  const cmd = mock.calls[0];
  assert.ok(cmd instanceof DeleteItemCommand);
  assert.equal(cmd.input.TableName, 'tbl');
  assert.equal(cmd.input.Key?.pk?.S, 'CACHE#a');
  assert.equal(cmd.input.Key?.sk?.S, 'LOCK');
}
