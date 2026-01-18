import test from 'node:test';
import assert from 'node:assert/strict';

import {
  CreateTableCommand,
  DeleteTableCommand,
  DescribeTableCommand,
  DynamoDBClient,
} from '@aws-sdk/client-dynamodb';

import { TheorydbClient } from '../../../../ts/src/client.js';
import { defineModel } from '../../../../ts/src/model.js';
import { pingDynamo } from '../src/runner.js';

function isResourceNotFound(err: unknown): boolean {
  return (
    typeof err === 'object' &&
    err !== null &&
    'name' in err &&
    (err as { name?: unknown }).name === 'ResourceNotFoundException'
  );
}

async function waitTableExists(ddb: DynamoDBClient, tableName: string): Promise<void> {
  for (let i = 0; i < 60; i++) {
    try {
      const resp = await ddb.send(
        new DescribeTableCommand({ TableName: tableName }),
      );
      if (resp.Table?.TableStatus === 'ACTIVE') return;
    } catch (err) {
      if (!isResourceNotFound(err)) throw err;
    }
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error(`timeout waiting for table exists: ${tableName}`);
}

async function waitTableNotExists(ddb: DynamoDBClient, tableName: string): Promise<void> {
  for (let i = 0; i < 40; i++) {
    try {
      await ddb.send(new DescribeTableCommand({ TableName: tableName }));
    } catch (err) {
      if (isResourceNotFound(err)) return;
      throw err;
    }
    await new Promise((r) => setTimeout(r, 150));
  }
}

async function recreateTable(ddb: DynamoDBClient, tableName: string): Promise<void> {
  try {
    await ddb.send(new DeleteTableCommand({ TableName: tableName }));
  } catch (err) {
    if (!isResourceNotFound(err)) throw err;
  }
  await waitTableNotExists(ddb, tableName);

  await ddb.send(
    new CreateTableCommand({
      TableName: tableName,
      BillingMode: 'PAY_PER_REQUEST',
      AttributeDefinitions: [
        { AttributeName: 'PK', AttributeType: 'S' },
        { AttributeName: 'SK', AttributeType: 'S' },
      ],
      KeySchema: [
        { AttributeName: 'PK', KeyType: 'HASH' },
        { AttributeName: 'SK', KeyType: 'RANGE' },
      ],
    }),
  );
  await waitTableExists(ddb, tableName);
}

test('filters + groups + projection escaping + cursor handoff', async (t) => {
  const endpoint = process.env.DYNAMODB_ENDPOINT ?? 'http://localhost:8000';
  const skipIntegration =
    process.env.SKIP_INTEGRATION === 'true' || process.env.SKIP_INTEGRATION === '1';

  const ddb = new DynamoDBClient({
    region: process.env.AWS_REGION ?? 'us-east-1',
    endpoint,
    credentials: {
      accessKeyId: process.env.AWS_ACCESS_KEY_ID ?? 'dummy',
      secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY ?? 'dummy',
    },
  });

  try {
    await pingDynamo(ddb);
  } catch (err) {
    if (skipIntegration) {
      t.skip(`DynamoDB Local not reachable (SKIP_INTEGRATION set; endpoint: ${endpoint})`);
      return;
    }
    throw err;
  }

  const tableName = 'filters_contract';
  await recreateTable(ddb, tableName);

  const model = defineModel({
    name: 'FilterContract',
    table: { name: tableName },
    keys: {
      partition: { attribute: 'PK', type: 'S' },
      sort: { attribute: 'SK', type: 'S' },
    },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'SK', type: 'S', roles: ['sk'] },
      { attribute: 'tag', type: 'S', optional: true, omit_empty: true },
      { attribute: 'name', type: 'S', optional: true, omit_empty: true },
    ],
  });

  const theorydb = new TheorydbClient(ddb).register(model);
  const pk = `PK#${Date.now()}`;

  await theorydb.create('FilterContract', { PK: pk, SK: '0' });
  await theorydb.create('FilterContract', { PK: pk, SK: '1', tag: 'X', name: 'Alice' });
  await theorydb.create('FilterContract', { PK: pk, SK: '2', tag: 'Y', name: 'Bob' });

  const q0 = await theorydb
    .query('FilterContract')
    .partitionKey(pk)
    .filter('tag', 'EXISTS')
    .limit(1)
    .page();
  assert.equal(q0.items.length, 0);
  assert.ok(q0.cursor);

  const q1 = await theorydb
    .query('FilterContract')
    .partitionKey(pk)
    .filter('tag', 'EXISTS')
    .limit(1)
    .cursor(q0.cursor!)
    .page();
  assert.equal(q1.items.length, 1);
  assert.equal(q1.items[0]!.SK, '1');

  const q2 = await theorydb
    .query('FilterContract')
    .partitionKey(pk)
    .projection(['PK', 'SK', 'name'])
    .filterGroup((f) => f.filter('tag', '=', 'X').orFilter('tag', '=', 'Y'))
    .page();
  assert.deepEqual(
    q2.items.map((i) => i.name).sort(),
    ['Alice', 'Bob'],
  );

  const scanned = await theorydb.scan('FilterContract').scanAllSegments(2, { concurrency: 1 });
  assert.equal(
    new Set(scanned.map((i) => i.SK)).size,
    3,
  );
});

