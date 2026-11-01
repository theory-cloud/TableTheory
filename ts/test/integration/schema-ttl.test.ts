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
import { TheorydbError } from '../../src/errors.js';
import { createTable, deleteTable, ensureTable } from '../../src/schema.js';

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

  // ensureTable must surface the same typed conflict for an existing table.
  let ensureConflictError: unknown;
  try {
    await ensureTable(ddb, ttlModel);
  } catch (err) {
    ensureConflictError = err;
  }
  assert.ok(ensureConflictError, 'expected ensureTable to reject the conflict');
  assert.ok(
    ensureConflictError instanceof TheorydbError,
    'ensureTable must raise a TheorydbError',
  );
  assert.equal(ensureConflictError.code, 'ErrInvalidModel');
  const ensureMessage = ensureConflictError.message;
  assert.match(
    ensureMessage,
    /expiresAt/,
    `ensureTable error must name the model attribute: ${ensureMessage}`,
  );
  assert.match(
    ensureMessage,
    /LegacyTtl/,
    `ensureTable error must name the enabled attribute: ${ensureMessage}`,
  );
  assert.match(
    ensureMessage,
    /different attribute/i,
    `ensureTable error must explain the conflict: ${ensureMessage}`,
  );

  await ddb.send(new DeleteTableCommand({ TableName: tableName }));

  // Provisioning must never enable TTL on an existing table: enabling it would
  // make DynamoDB expire items that are already past their expiry.
  await ddb.send(
    new CreateTableCommand({
      TableName: tableName,
      BillingMode: 'PAY_PER_REQUEST',
      KeySchema: [{ AttributeName: 'ID', KeyType: 'HASH' }],
      AttributeDefinitions: [{ AttributeName: 'ID', AttributeType: 'S' }],
    }),
  );

  const warnings: string[] = [];
  const originalWarn = console.warn;
  console.warn = (message?: unknown) => {
    warnings.push(String(message));
  };
  try {
    await ensureTable(ddb, ttlModel);
    await createTable(ddb, ttlModel);
  } finally {
    console.warn = originalWarn;
  }

  const afterEnsure = await ddb.send(
    new DescribeTimeToLiveCommand({ TableName: tableName }),
  );
  assert.equal(
    afterEnsure.TimeToLiveDescription?.TimeToLiveStatus,
    'DISABLED',
    'provisioning must leave TTL disabled on an existing table',
  );
  assert.equal(
    afterEnsure.TimeToLiveDescription?.AttributeName,
    undefined,
    'provisioning must not claim a TTL attribute on a disabled table',
  );
  assert.match(
    warnings.join('\n'),
    /leaving TTL disabled/,
    'the disabled TTL must be reported as a warning',
  );

  await ddb.send(new DeleteTableCommand({ TableName: tableName }));
} finally {
  ddb.destroy();
}
