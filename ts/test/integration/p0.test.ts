import assert from 'node:assert/strict';

import {
  CreateTableCommand,
  DeleteItemCommand,
  DescribeTableCommand,
  DynamoDBClient,
  GetItemCommand,
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
        `Skipping TS P0 integration tests (SKIP_INTEGRATION set; endpoint: ${endpoint})`,
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
      { attribute: 'tags', type: 'SS', optional: true, omit_empty: true },
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
      {
        attribute: 'ttl',
        type: 'N',
        format: 'unix_seconds',
        roles: ['ttl'],
        optional: true,
        omit_empty: true,
      },
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

  const id = `USER#ts-${Date.now()}`;
  const key = { PK: id, SK: 'PROFILE' };

  // p0.crud.basic
  await theorydb.create(
    'User',
    { ...key, emailHash: 'hash_abc', nickname: 'Al', tags: ['a', 'b'] },
    { ifNotExists: true },
  );
  await assert.rejects(
    () =>
      theorydb.create(
        'User',
        { ...key, emailHash: 'hash_abc' },
        { ifNotExists: true },
      ),
    (err) => err instanceof TheorydbError && err.code === 'ErrConditionFailed',
  );

  const got0 = await theorydb.get('User', key);
  assert.equal(got0.PK, id);
  assert.equal(got0.SK, 'PROFILE');
  assert.equal(got0.emailHash, 'hash_abc');
  assert.equal(got0.nickname, 'Al');
  assert.deepEqual(got0.tags, ['a', 'b']);
  assert.equal(typeof got0.createdAt, 'string');
  assert.equal(typeof got0.updatedAt, 'string');
  assert.equal(got0.version, 0);

  await theorydb.update('User', { ...key, nickname: 'Alice', version: 0 }, [
    'nickname',
  ]);
  const got1 = await theorydb.get('User', key);
  assert.equal(got1.nickname, 'Alice');
  assert.equal(got1.version, 1);

  await theorydb.delete('User', key);
  await assert.rejects(
    () => theorydb.get('User', key),
    (err) => err instanceof TheorydbError && err.code === 'ErrItemNotFound',
  );

  // p0.encoding.omitempty
  const id2 = `${id}#omitempty`;
  const key2 = { PK: id2, SK: 'PROFILE' };
  await theorydb.create('User', { ...key2, nickname: '', tags: [], ttl: 0 });
  const got2 = await theorydb.get('User', key2);
  assert.ok(!('nickname' in got2));
  assert.ok(!('tags' in got2));
  assert.ok(!('ttl' in got2));
  assert.equal(typeof got2.createdAt, 'string');
  assert.equal(typeof got2.updatedAt, 'string');
  assert.equal(got2.version, 0);

  // p0.lifecycle.created_updated
  const id3 = `${id}#lifecycle`;
  const key3 = { PK: id3, SK: 'PROFILE' };
  await theorydb.create('User', { ...key3, nickname: 'v0' });
  const got3a = await theorydb.get('User', key3);
  assert.equal(got3a.nickname, 'v0');
  const createdAt0 = got3a.createdAt;
  const updatedAt0 = got3a.updatedAt;
  assert.equal(got3a.version, 0);

  await sleep(25);
  await theorydb.update('User', { ...key3, nickname: 'v1', version: 0 }, [
    'nickname',
  ]);
  const got3b = await theorydb.get('User', key3);
  assert.equal(got3b.createdAt, createdAt0);
  assert.notEqual(got3b.updatedAt, updatedAt0);
  assert.equal(got3b.version, 1);

  // p0.lifecycle.version_optimistic_lock
  const id4 = `${id}#version`;
  const key4 = { PK: id4, SK: 'PROFILE' };
  await theorydb.create('User', { ...key4, nickname: 'v0' });
  await theorydb.update('User', { ...key4, nickname: 'v1', version: 0 }, [
    'nickname',
  ]);
  await assert.rejects(
    () =>
      theorydb.update('User', { ...key4, nickname: 'stale', version: 0 }, [
        'nickname',
      ]),
    (err) => err instanceof TheorydbError && err.code === 'ErrConditionFailed',
  );
  const got4 = await theorydb.get('User', key4);
  assert.equal(got4.nickname, 'v1');
  assert.equal(got4.version, 1);

  // p0.lifecycle.ttl_epoch_seconds
  const id5 = `${id}#ttl`;
  const key5 = { PK: id5, SK: 'PROFILE' };
  await theorydb.create('User', { ...key5, ttl: 1_700_000_000 });
  const got5 = await theorydb.get('User', key5);
  assert.equal(got5.ttl, 1_700_000_000);

  const raw = await ddb.send(
    new GetItemCommand({
      TableName: user.tableName,
      Key: { PK: { S: id5 }, SK: { S: 'PROFILE' } },
    }),
  );
  assert.equal(raw.Item?.ttl ? Object.keys(raw.Item.ttl)[0] : undefined, 'N');

  // Cleanup one of the created items (best-effort).
  await ddb.send(
    new DeleteItemCommand({
      TableName: user.tableName,
      Key: { PK: { S: id2 }, SK: { S: 'PROFILE' } },
    }),
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

async function sleep(ms: number): Promise<void> {
  await new Promise((r) => setTimeout(r, ms));
}
