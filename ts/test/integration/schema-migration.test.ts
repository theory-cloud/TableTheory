import assert from 'node:assert/strict';

import {
  DynamoDBClient,
  ListTablesCommand,
  PutItemCommand,
  ScanCommand,
} from '@aws-sdk/client-dynamodb';

import { defineModel } from '../../src/model.js';
import { deleteTable, ensureTable } from '../../src/schema.js';
import {
  addField,
  autoMigrate,
  chainTransforms,
  renameField,
} from '../../src/schema-migration.js';

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

const userV1 = defineModel({
  name: 'UserV1',
  table: { name: 'users_migration_v1' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', required: true, roles: ['pk'] },
    { attribute: 'SK', type: 'S', required: true, roles: ['sk'] },
    { attribute: 'name', type: 'S' },
  ],
});

const userV2 = defineModel({
  name: 'UserV2',
  table: { name: 'users_migration_v2' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', required: true, roles: ['pk'] },
    { attribute: 'SK', type: 'S', required: true, roles: ['sk'] },
    { attribute: 'displayName', type: 'S' },
    { attribute: 'status', type: 'S' },
  ],
});

const backupTable = 'users_migration_backup';

try {
  try {
    await ddb.send(new ListTablesCommand({ Limit: 1 }));
  } catch (err) {
    if (skipIntegration) {
      console.warn(
        `Skipping schema-migration integration test (SKIP_INTEGRATION set; endpoint: ${endpoint})`,
      );
      process.exit(0);
    }
    throw err;
  }

  // Start from a clean slate so reruns are deterministic.
  await deleteTable(ddb, userV1, { ignoreMissing: true });
  await deleteTable(ddb, userV2, { ignoreMissing: true });
  await deleteTable(ddb, userV1, {
    tableName: backupTable,
    ignoreMissing: true,
  });

  await ensureTable(ddb, userV1);
  const seed: Array<[string, string]> = [
    ['USER#1', 'Ada'],
    ['USER#2', 'Grace'],
  ];
  for (const [pk, name] of seed) {
    await ddb.send(
      new PutItemCommand({
        TableName: userV1.tableName,
        Item: { PK: { S: pk }, SK: { S: 'v1' }, name: { S: name } },
      }),
    );
  }

  // Migrate v1 -> v2: back up the source, rename `name` -> `displayName`, add `status`.
  await autoMigrate(ddb, userV1, {
    targetModel: userV2,
    dataCopy: true,
    backupTable,
    transform: chainTransforms(
      renameField('name', 'displayName'),
      addField('status', { S: 'active' }),
    ),
  });

  const migrated = await ddb.send(
    new ScanCommand({ TableName: userV2.tableName }),
  );
  assert.equal(
    migrated.Items?.length,
    2,
    'v2 table should have both migrated items',
  );
  for (const it of migrated.Items ?? []) {
    assert.ok(it.displayName?.S, 'migrated item has displayName');
    assert.equal(it.status?.S, 'active', 'migrated item has status=active');
    assert.equal('name' in it, false, 'migrated item no longer has name');
  }

  const backup = await ddb.send(new ScanCommand({ TableName: backupTable }));
  assert.equal(
    backup.Items?.length,
    2,
    'backup table preserves the source items',
  );
  for (const it of backup.Items ?? []) {
    assert.ok(it.name?.S, 'backup item keeps the original name attribute');
  }

  await deleteTable(ddb, userV1, { ignoreMissing: true });
  await deleteTable(ddb, userV2, { ignoreMissing: true });
  await deleteTable(ddb, userV1, {
    tableName: backupTable,
    ignoreMissing: true,
  });

  console.log('schema-migration integration test passed');
} catch (err) {
  console.error(err);
  process.exit(1);
}
