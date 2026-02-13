import assert from 'node:assert/strict';

import {
  CreateTableCommand,
  DescribeTableCommand,
  DynamoDBClient,
  ResourceInUseException,
} from '@aws-sdk/client-dynamodb';

import { TheorydbError } from '../../src/errors.js';
import { FaceTheoryIsrMetaStore } from '../../src/facetheory-isr.js';

const endpoint = process.env.DYNAMODB_ENDPOINT ?? 'http://localhost:8000';

const ddb = new DynamoDBClient({
  region: process.env.AWS_REGION ?? 'us-east-1',
  endpoint,
  credentials: {
    accessKeyId: process.env.AWS_ACCESS_KEY_ID ?? 'dummy',
    secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY ?? 'dummy',
  },
});

const tableName = 'facetheory_isr_contract';

try {
  await ensureIsrTable(ddb, tableName);

  {
    const cacheKey = `ts-${Date.now()}`;

    const store1 = new FaceTheoryIsrMetaStore({
      ddb,
      tableName,
      leaseToken: () => 'tok1',
      leaseTtlBufferSeconds: 10,
    });

    const store2 = new FaceTheoryIsrMetaStore({
      ddb,
      tableName,
      leaseToken: () => 'tok2',
      leaseTtlBufferSeconds: 10,
    });

    const lease1 = await store1.tryAcquireLease({
      cacheKey,
      nowMs: 1_000_000,
      leaseDurationMs: 30_000,
    });
    assert.ok(lease1);
    assert.equal(lease1.leaseToken, 'tok1');

    const lease2 = await store2.tryAcquireLease({
      cacheKey,
      nowMs: 1_000_000,
      leaseDurationMs: 30_000,
    });
    assert.equal(lease2, null);

    await store1.commitGeneration({
      cacheKey,
      leaseToken: 'tok1',
      nowMs: 1_000_000,
      htmlPointer: 's3://bucket/key.html',
      generatedAtMs: 1_000_000,
      revalidateSeconds: 60,
      etag: '"abc"',
    });

    const got = await store1.get({ cacheKey });
    assert.deepEqual(got, {
      htmlPointer: 's3://bucket/key.html',
      generatedAtMs: 1_000_000,
      revalidateSeconds: 60,
      etag: '"abc"',
    });

    // Commit publishes META and releases the LOCK, so another contender can acquire immediately.
    const lease3 = await store2.tryAcquireLease({
      cacheKey,
      nowMs: 1_000_000,
      leaseDurationMs: 30_000,
    });
    assert.ok(lease3);
    assert.equal(lease3.leaseToken, 'tok2');
  }

  {
    const cacheKey = `ts-stale-${Date.now()}`;

    const store1 = new FaceTheoryIsrMetaStore({
      ddb,
      tableName,
      leaseToken: () => 'tok1',
      leaseTtlBufferSeconds: 10,
    });

    const store2 = new FaceTheoryIsrMetaStore({
      ddb,
      tableName,
      leaseToken: () => 'tok2',
      leaseTtlBufferSeconds: 10,
    });

    const lease1 = await store1.tryAcquireLease({
      cacheKey,
      nowMs: 1_000_000,
      leaseDurationMs: 30_000,
    });
    assert.ok(lease1);
    assert.equal(lease1.leaseToken, 'tok1');

    // After expiry, a new contender can acquire.
    const lease2 = await store2.tryAcquireLease({
      cacheKey,
      nowMs: 2_000_000,
      leaseDurationMs: 30_000,
    });
    assert.ok(lease2);
    assert.equal(lease2.leaseToken, 'tok2');

    await assert.rejects(
      () =>
        store1.commitGeneration({
          cacheKey,
          leaseToken: 'tok1',
          nowMs: 2_000_000,
          htmlPointer: 's3://bucket/stale.html',
          generatedAtMs: 2_000_000,
          revalidateSeconds: 60,
        }),
      (e) => e instanceof TheorydbError && e.code === 'ErrLeaseNotOwned',
    );
  }
} finally {
  ddb.destroy();
}

async function ensureIsrTable(
  client: DynamoDBClient,
  name: string,
): Promise<void> {
  try {
    await client.send(new DescribeTableCommand({ TableName: name }));
    return;
  } catch {
    // continue
  }

  try {
    await client.send(
      new CreateTableCommand({
        TableName: name,
        AttributeDefinitions: [
          { AttributeName: 'pk', AttributeType: 'S' },
          { AttributeName: 'sk', AttributeType: 'S' },
        ],
        KeySchema: [
          { AttributeName: 'pk', KeyType: 'HASH' },
          { AttributeName: 'sk', KeyType: 'RANGE' },
        ],
        ProvisionedThroughput: { ReadCapacityUnits: 1, WriteCapacityUnits: 1 },
      }),
    );
  } catch (err) {
    if (err instanceof ResourceInUseException) return;
    if (
      typeof err === 'object' &&
      err !== null &&
      'name' in err &&
      (err as { name?: unknown }).name === 'ResourceInUseException'
    ) {
      return;
    }
    throw err;
  }
}
