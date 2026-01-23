import assert from 'node:assert/strict';

import {
  CreateTableCommand,
  DescribeTableCommand,
  DynamoDBClient,
  ResourceInUseException,
} from '@aws-sdk/client-dynamodb';

import { TheorydbError } from '../../src/errors.js';
import { LeaseManager } from '../../src/lease.js';

const endpoint = process.env.DYNAMODB_ENDPOINT ?? 'http://localhost:8000';

const ddb = new DynamoDBClient({
  region: process.env.AWS_REGION ?? 'us-east-1',
  endpoint,
  credentials: {
    accessKeyId: process.env.AWS_ACCESS_KEY_ID ?? 'dummy',
    secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY ?? 'dummy',
  },
});

try {
  await ensureLeaseTable(ddb);

  const pk = `CACHE#ts-${Date.now()}`;
  const key = { pk, sk: 'LOCK' };

  const mgr1 = new LeaseManager(ddb, 'lease_contract', {
    now: () => 1000,
    token: () => 'tok1',
    ttlBufferSeconds: 10,
  });

  const mgr2 = new LeaseManager(ddb, 'lease_contract', {
    now: () => 1000,
    token: () => 'tok2',
    ttlBufferSeconds: 10,
  });

  const l1 = await mgr1.acquire(key, { leaseSeconds: 30 });
  assert.equal(l1.token, 'tok1');

  await assert.rejects(
    () => mgr2.acquire(key, { leaseSeconds: 30 }),
    (e) => e instanceof TheorydbError && e.code === 'ErrLeaseHeld',
  );

  const mgr2Late = new LeaseManager(ddb, 'lease_contract', {
    now: () => 2000,
    token: () => 'tok2',
    ttlBufferSeconds: 10,
  });

  const l2 = await mgr2Late.acquire(key, { leaseSeconds: 30 });
  assert.equal(l2.token, 'tok2');

  await assert.rejects(
    () => mgr1.refresh(l1, { leaseSeconds: 30 }),
    (e) => e instanceof TheorydbError && e.code === 'ErrLeaseNotOwned',
  );
} finally {
  ddb.destroy();
}

async function ensureLeaseTable(client: DynamoDBClient): Promise<void> {
  const tableName = 'lease_contract';
  try {
    await client.send(new DescribeTableCommand({ TableName: tableName }));
    return;
  } catch {
    // continue
  }

  try {
    await client.send(
      new CreateTableCommand({
        TableName: tableName,
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
