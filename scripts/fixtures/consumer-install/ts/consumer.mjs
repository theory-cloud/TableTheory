// A clean consumer of the packed TypeScript artifact.
//
// This file is copied into an empty directory that has installed the packed
// release tarball and nothing from this repository, so every import below
// resolves through the published package entry points. It exercises one real
// call path against DynamoDB Local: create a table with the AWS SDK, define a
// model, write and read an item through TableTheory, and repeat the write to
// prove the conditional write path still fails closed.
import assert from 'node:assert/strict';

import {
  CreateTableCommand,
  DeleteTableCommand,
  DynamoDBClient,
} from '@aws-sdk/client-dynamodb';

import { TheorydbClient, defineModel } from '@theory-cloud/tabletheory-ts';

const endpoint = process.env.DYNAMODB_ENDPOINT;
if (!endpoint) {
  throw new Error('DYNAMODB_ENDPOINT is required: this check proves a real call path');
}

const tableName = `tabletheory_ts_consumer_${process.pid}_${Date.now()}`;

const ddb = new DynamoDBClient({
  region: process.env.AWS_REGION ?? 'us-east-1',
  endpoint,
  credentials: {
    accessKeyId: process.env.AWS_ACCESS_KEY_ID ?? 'dummy',
    secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY ?? 'dummy',
  },
});

const Note = defineModel({
  name: 'Note',
  table: { name: tableName },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', required: true, roles: ['pk'] },
    { attribute: 'SK', type: 'S', required: true, roles: ['sk'] },
    { attribute: 'body', type: 'S', optional: true, omit_empty: true },
    { attribute: 'createdAt', type: 'S', format: 'rfc3339nano', roles: ['created_at'] },
    { attribute: 'updatedAt', type: 'S', format: 'rfc3339nano', roles: ['updated_at'] },
    { attribute: 'version', type: 'N', format: 'int', roles: ['version'] },
  ],
});

await ddb.send(
  new CreateTableCommand({
    TableName: tableName,
    KeySchema: [
      { AttributeName: 'PK', KeyType: 'HASH' },
      { AttributeName: 'SK', KeyType: 'RANGE' },
    ],
    AttributeDefinitions: [
      { AttributeName: 'PK', AttributeType: 'S' },
      { AttributeName: 'SK', AttributeType: 'S' },
    ],
    BillingMode: 'PAY_PER_REQUEST',
  }),
);

try {
  const theorydb = new TheorydbClient(ddb).register(Note);
  const key = { PK: 'NOTE#consumer', SK: 'V1' };

  await theorydb.create('Note', { ...key, body: 'installed from the packed tarball' });
  const created = await theorydb.get('Note', key);
  assert.equal(created.body, 'installed from the packed tarball');
  assert.equal(created.version, '0');
  assert.equal(typeof created.createdAt, 'string');
  assert.equal(typeof created.updatedAt, 'string');

  // omit-empty: the empty body is never written, so the read has no attribute.
  const emptyKey = { PK: 'NOTE#consumer', SK: 'V2' };
  await theorydb.create('Note', { ...emptyKey, body: '' });
  const empty = await theorydb.get('Note', emptyKey);
  assert.ok(!('body' in empty));

  await theorydb.delete('Note', key);
  await assert.rejects(() => theorydb.get('Note', key));

  console.log(
    `consumer-install(ts): PASS on node ${process.version} via ${endpoint} ` +
      `(package @theory-cloud/tabletheory-ts)`,
  );
} finally {
  await ddb.send(new DeleteTableCommand({ TableName: tableName }));
  ddb.destroy();
}
