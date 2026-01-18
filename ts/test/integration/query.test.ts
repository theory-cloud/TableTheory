import assert from 'node:assert/strict';

import {
  CreateTableCommand,
  DescribeTableCommand,
  DynamoDBClient,
  ListTablesCommand,
  ResourceInUseException,
} from '@aws-sdk/client-dynamodb';

import { TheorydbClient } from '../../src/client.js';
import { TheorydbError } from '../../src/errors.js';
import { defineModel } from '../../src/model.js';

const endpoint = process.env.DYNAMODB_ENDPOINT ?? 'http://localhost:8000';
const skipIntegration =
  process.env.SKIP_INTEGRATION === 'true' ||
  process.env.SKIP_INTEGRATION === '1';

const ddb = new DynamoDBClient({
  region: process.env.AWS_REGION ?? 'us-east-1',
  endpoint,
  credentials: {
    accessKeyId: process.env.AWS_ACCESS_KEY_ID ?? 'dummy',
    secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY ?? 'dummy',
  },
});

try {
  try {
    await ddb.send(new ListTablesCommand({ Limit: 1 }));
  } catch (err) {
    if (skipIntegration) {
      console.warn(
        `Skipping query integration tests (SKIP_INTEGRATION set; endpoint: ${endpoint})`,
      );
      process.exit(0);
    }
    throw err;
  }
  await ensureUsersTable(ddb);

  const user = defineModel({
    name: 'User',
    table: { name: 'users_contract' },
    naming: { convention: 'camelCase' },
    keys: {
      partition: { attribute: 'PK', type: 'S' },
      sort: { attribute: 'SK', type: 'S' },
    },
    attributes: [
      { attribute: 'PK', type: 'S', required: true, roles: ['pk'] },
      { attribute: 'SK', type: 'S', required: true, roles: ['sk'] },
      { attribute: 'emailHash', type: 'S', optional: true },
      { attribute: 'nickname', type: 'S', optional: true, omit_empty: true },
      {
        attribute: 'createdAt',
        type: 'S',
        format: 'rfc3339nano',
        roles: ['created_at'],
      },
      {
        attribute: 'updatedAt',
        type: 'S',
        format: 'rfc3339nano',
        roles: ['updated_at'],
      },
      { attribute: 'version', type: 'N', format: 'int', roles: ['version'] },
    ],
    indexes: [
      {
        name: 'gsi-email',
        type: 'GSI',
        partition: { attribute: 'emailHash', type: 'S' },
        projection: { type: 'ALL' },
      },
    ],
  });

  const theorydb = new TheorydbClient(ddb).register(user);

  const pk = `USER#query-${Date.now()}`;
  await theorydb.create(
    'User',
    { PK: pk, SK: 'A', emailHash: 'hash_email' },
    { ifNotExists: true },
  );
  await theorydb.create(
    'User',
    { PK: pk, SK: 'B', emailHash: 'hash_email', nickname: 'Alice' },
    { ifNotExists: true },
  );
  await theorydb.create(
    'User',
    { PK: pk, SK: 'C', emailHash: 'hash_email', nickname: 'Bob' },
    { ifNotExists: true },
  );

  const page1 = await theorydb.query('User').partitionKey(pk).limit(2).page();
  assert.equal(page1.items.length, 2);
  assert.ok(page1.cursor);
  assert.deepEqual(
    page1.items.map((i) => i.SK),
    ['A', 'B'],
  );

  const page2 = await theorydb
    .query('User')
    .partitionKey(pk)
    .limit(2)
    .cursor(page1.cursor!)
    .page();
  assert.equal(page2.items.length, 1);
  assert.equal(page2.items[0]!.SK, 'C');
  assert.equal(page2.cursor, undefined);

  const desc = await theorydb
    .query('User')
    .partitionKey(pk)
    .sort('DESC')
    .limit(1)
    .page();
  assert.equal(desc.items.length, 1);
  assert.equal(desc.items[0]!.SK, 'C');

  const proj = await theorydb
    .query('User')
    .partitionKey(pk)
    .projection(['PK', 'SK'])
    .limit(1)
    .page();
  assert.equal(proj.items.length, 1);
  assert.ok('PK' in proj.items[0]!);
  assert.ok('SK' in proj.items[0]!);
  assert.ok(!('createdAt' in proj.items[0]!));

  const byEmail = await theorydb
    .query('User')
    .usingIndex('gsi-email')
    .partitionKey('hash_email')
    .limit(1)
    .page();
  assert.ok(byEmail.items.length >= 1);
  assert.ok(byEmail.cursor);

  await assert.rejects(
    () =>
      theorydb.query('User').partitionKey(pk).cursor(byEmail.cursor!).page(),
    (err) => err instanceof TheorydbError && err.code === 'ErrInvalidOperator',
  );

  const filtered0 = await theorydb
    .query('User')
    .partitionKey(pk)
    .filter('nickname', 'EXISTS')
    .limit(1)
    .page();
  assert.equal(filtered0.items.length, 0);
  assert.ok(filtered0.cursor);

  const filtered1 = await theorydb
    .query('User')
    .partitionKey(pk)
    .filter('nickname', 'EXISTS')
    .limit(1)
    .cursor(filtered0.cursor!)
    .page();
  assert.equal(filtered1.items.length, 1);
  assert.equal(filtered1.items[0]!.SK, 'B');
  assert.ok(filtered1.cursor);

  const filtered2 = await theorydb
    .query('User')
    .partitionKey(pk)
    .filterGroup((f) =>
      f.filter('nickname', '=', 'Alice').orFilter('nickname', '=', 'Bob'),
    )
    .page();
  assert.deepEqual(
    filtered2.items.map((i) => i.SK),
    ['B', 'C'],
  );
} finally {
  ddb.destroy();
}

async function ensureUsersTable(client: DynamoDBClient): Promise<void> {
  const tableName = 'users_contract';
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
          { AttributeName: 'PK', AttributeType: 'S' },
          { AttributeName: 'SK', AttributeType: 'S' },
          { AttributeName: 'emailHash', AttributeType: 'S' },
        ],
        KeySchema: [
          { AttributeName: 'PK', KeyType: 'HASH' },
          { AttributeName: 'SK', KeyType: 'RANGE' },
        ],
        GlobalSecondaryIndexes: [
          {
            IndexName: 'gsi-email',
            KeySchema: [{ AttributeName: 'emailHash', KeyType: 'HASH' }],
            Projection: { ProjectionType: 'ALL' },
            ProvisionedThroughput: {
              ReadCapacityUnits: 1,
              WriteCapacityUnits: 1,
            },
          },
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
