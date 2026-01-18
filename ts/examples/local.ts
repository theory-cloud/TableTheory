import { randomUUID } from 'node:crypto';

import {
  CreateTableCommand,
  DescribeTableCommand,
  DynamoDBClient,
  ResourceInUseException,
} from '@aws-sdk/client-dynamodb';

import { TheorydbClient, defineModel } from '../src/index.js';

const endpoint = process.env.DYNAMODB_ENDPOINT ?? 'http://localhost:8000';
const region = process.env.AWS_REGION ?? 'us-east-1';

const ddb = new DynamoDBClient({
  region,
  endpoint,
  credentials: {
    accessKeyId: process.env.AWS_ACCESS_KEY_ID ?? 'dummy',
    secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY ?? 'dummy',
  },
});

await ensureUsersTable(ddb);

const User = defineModel({
  name: 'User',
  table: { name: 'users_contract' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'emailHash', type: 'S', optional: true },
    { attribute: 'nickname', type: 'S', optional: true, omit_empty: true },
    { attribute: 'createdAt', type: 'S', roles: ['created_at'] },
    { attribute: 'updatedAt', type: 'S', roles: ['updated_at'] },
    { attribute: 'version', type: 'N', roles: ['version'] },
    { attribute: 'ttl', type: 'N', roles: ['ttl'], optional: true },
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

const db = new TheorydbClient(ddb).register(User);

const pk = `USER#${randomUUID()}`;
const key = { PK: pk, SK: 'PROFILE' };

await db.create(
  'User',
  { ...key, nickname: 'Alice', emailHash: 'hash_email', ttl: 1_700_000_000 },
  { ifNotExists: true },
);

const created = await db.get('User', key);
console.log('created:', created);

const page1 = await db.query('User').partitionKey(pk).limit(1).page();
console.log('query page1:', page1.items);

if (page1.cursor) {
  const page2 = await db
    .query('User')
    .partitionKey(pk)
    .cursor(page1.cursor)
    .page();
  console.log('query page2:', page2.items);
}

await db.update(
  'User',
  { ...key, nickname: 'Alicia', version: created.version as number },
  ['nickname'],
);

const updated = await db.get('User', key);
console.log('updated:', updated);

await db.delete('User', key);
console.log('deleted');

ddb.destroy();

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

  for (let attempt = 0; attempt < 25; attempt += 1) {
    const described = await client.send(
      new DescribeTableCommand({ TableName: tableName }),
    );
    if (described.Table?.TableStatus === 'ACTIVE') return;
    await new Promise((resolve) => {
      setTimeout(resolve, 200);
    });
  }
}
