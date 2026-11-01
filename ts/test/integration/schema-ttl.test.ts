import assert from 'node:assert/strict';

import {
  CreateTableCommand,
  DeleteTableCommand,
  DescribeTimeToLiveCommand,
  DynamoDBClient,
  ListTablesCommand,
  UpdateTimeToLiveCommand,
} from '@aws-sdk/client-dynamodb';

import { defineModel } from '../../src/model.js';
import { createTable, deleteTable } from '../../src/schema.js';

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

const tableName = 'tabletheory_local_t15_ttl_create_ts';

const ttlModel = defineModel({
  name: 'TtlCreateTableModel',
  table: { name: tableName },
  keys: {
    partition: { attribute: 'ID', type: 'S' },
  },
  attributes: [
    { attribute: 'ID', type: 'S', required: true, roles: ['pk'] },
    { attribute: 'expiresAt', type: 'N', roles: ['ttl'] },
  ],
});

try {
  try {
    await ddb.send(new ListTablesCommand({ Limit: 1 }));
  } catch (err) {
    if (skipIntegration) {
      console.warn(
        `Skipping schema-ttl integration test (SKIP_INTEGRATION set; endpoint: ${endpoint})`,
      );
      process.exit(0);
    }
    throw err;
  }

  await deleteTable(ddb, ttlModel, { ignoreMissing: true });

  // First create provisions the table and enables TTL on the model attribute.
  await createTable(ddb, ttlModel);

  // A second CreateTable on the existing, TTL-enabled table must behave like
  // the already-exists outcome of a table without TTL: no error.
  await createTable(ddb, ttlModel);

  const afterRepeat = await ddb.send(
    new DescribeTimeToLiveCommand({ TableName: tableName }),
  );
  assert.equal(
    afterRepeat.TimeToLiveDescription?.TimeToLiveStatus,
    'ENABLED',
    'TTL must stay enabled after a repeated CreateTable',
  );
  assert.equal(
    afterRepeat.TimeToLiveDescription?.AttributeName,
    'expiresAt',
    'TTL must stay on the model attribute after a repeated CreateTable',
  );

  await deleteTable(ddb, ttlModel, { ignoreMissing: true });

  // A table whose TTL is enabled on a different attribute must surface a
  // clear error naming both attributes instead of being silently accepted.
  await ddb.send(
    new CreateTableCommand({
      TableName: tableName,
      BillingMode: 'PAY_PER_REQUEST',
      KeySchema: [{ AttributeName: 'ID', KeyType: 'HASH' }],
      AttributeDefinitions: [{ AttributeName: 'ID', AttributeType: 'S' }],
    }),
  );
  await ddb.send(
    new UpdateTimeToLiveCommand({
      TableName: tableName,
      TimeToLiveSpecification: {
        AttributeName: 'LegacyTtl',
        Enabled: true,
      },
    }),
  );

  let conflictError: unknown;
  try {
    await createTable(ddb, ttlModel);
  } catch (err) {
    conflictError = err;
  }
  assert.ok(conflictError, 'expected a conflict error');
  const message = (conflictError as Error).message;
  assert.match(
    message,
    /expiresAt/,
    `error must name the model attribute: ${message}`,
  );
  assert.match(
    message,
    /LegacyTtl/,
    `error must name the enabled attribute: ${message}`,
  );
  assert.match(
    message,
    /different attribute/i,
    `error must explain the conflict: ${message}`,
  );

  await ddb.send(new DeleteTableCommand({ TableName: tableName }));
} finally {
  ddb.destroy();
}
