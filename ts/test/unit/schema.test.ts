import assert from 'node:assert/strict';
import test from 'node:test';

import {
  CreateTableCommand,
  DeleteTableCommand,
  DescribeTableCommand,
  type DynamoDBClient,
  UpdateTimeToLiveCommand,
} from '@aws-sdk/client-dynamodb';

import { TheorydbError } from '../../src/errors.js';
import type { ModelSchema } from '../../src/model.js';
import { defineModel } from '../../src/model.js';
import {
  createTable,
  deleteTable,
  describeTable,
  ensureTable,
} from '../../src/schema.js';

const baseSchema: ModelSchema = {
  name: 'User',
  table: { name: 'users_contract' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', required: true, roles: ['pk'] },
    { attribute: 'SK', type: 'S', required: true, roles: ['sk'] },
    { attribute: 'emailHash', type: 'S', optional: true },
  ],
  indexes: [
    {
      name: 'gsi-email',
      type: 'GSI',
      partition: { attribute: 'emailHash', type: 'S' },
      projection: { type: 'ALL' },
    },
  ],
};

const ttlSchema: ModelSchema = {
  ...baseSchema,
  attributes: [
    ...baseSchema.attributes,
    {
      attribute: 'expiresAt',
      type: 'N',
      optional: true,
      roles: ['ttl'],
    },
  ],
};

void test('createTable sends CreateTableCommand with model schema', async () => {
  const model = defineModel(baseSchema);
  const calls: unknown[] = [];
  const ddb = {
    send: async (cmd: unknown) => {
      calls.push(cmd);
      return {};
    },
  } as unknown as DynamoDBClient;

  await createTable(ddb, model, { waitForActive: false });

  assert.equal(calls.length, 1);
  const cmd = calls[0];
  assert.ok(cmd instanceof CreateTableCommand);
  assert.equal(cmd.input.TableName, 'users_contract');
  assert.equal(cmd.input.BillingMode, 'PAY_PER_REQUEST');
  assert.deepEqual(cmd.input.KeySchema, [
    { AttributeName: 'PK', KeyType: 'HASH' },
    { AttributeName: 'SK', KeyType: 'RANGE' },
  ]);
  assert.deepEqual(cmd.input.AttributeDefinitions, [
    { AttributeName: 'PK', AttributeType: 'S' },
    { AttributeName: 'SK', AttributeType: 'S' },
    { AttributeName: 'emailHash', AttributeType: 'S' },
  ]);
  assert.equal(cmd.input.GlobalSecondaryIndexes?.length, 1);
  assert.equal(cmd.input.LocalSecondaryIndexes, undefined);
});

void test('ensureTable creates when DescribeTable is missing', async () => {
  const model = defineModel(baseSchema);
  const calls: unknown[] = [];
  const ddb = {
    send: async (cmd: unknown) => {
      calls.push(cmd);
      if (cmd instanceof DescribeTableCommand) {
        throw { name: 'ResourceNotFoundException' };
      }
      return {};
    },
  } as unknown as DynamoDBClient;

  await ensureTable(ddb, model, { waitForActive: false });
  assert.equal(calls.length, 2);
  assert.ok(calls[0] instanceof DescribeTableCommand);
  assert.ok(calls[1] instanceof CreateTableCommand);
});

void test('describeTable maps ResourceNotFoundException to ErrTableNotFound', async () => {
  const model = defineModel(baseSchema);
  const ddb = {
    send: async () => {
      throw { name: 'ResourceNotFoundException' };
    },
  } as unknown as DynamoDBClient;

  await assert.rejects(
    async () => describeTable(ddb, model),
    (err) => {
      assert.ok(err instanceof TheorydbError);
      assert.equal(err.code, 'ErrTableNotFound');
      return true;
    },
  );
});

void test('deleteTable ignores missing tables when ignoreMissing=true', async () => {
  const model = defineModel(baseSchema);
  const calls: unknown[] = [];
  const ddb = {
    send: async (cmd: unknown) => {
      calls.push(cmd);
      throw { name: 'ResourceNotFoundException' };
    },
  } as unknown as DynamoDBClient;

  await deleteTable(ddb, model, { waitForDelete: false, ignoreMissing: true });
  assert.equal(calls.length, 1);
  assert.ok(calls[0] instanceof DeleteTableCommand);
});

void test('createTable treats ResourceInUseException as ok', async () => {
  const model = defineModel(baseSchema);
  const ddb = {
    send: async (cmd: unknown) => {
      if (cmd instanceof CreateTableCommand)
        throw { name: 'ResourceInUseException' };
      return {};
    },
  } as unknown as DynamoDBClient;

  await createTable(ddb, model, { waitForActive: false });
});

void test('createTable rejects invalid LSI partition keys early', async () => {
  const model = defineModel({
    ...baseSchema,
    indexes: [
      {
        name: 'lsi-bad',
        type: 'LSI',
        partition: { attribute: 'emailHash', type: 'S' },
        sort: { attribute: 'SK', type: 'S' },
      },
    ],
  });
  const ddb = { send: async () => ({}) } as unknown as DynamoDBClient;

  await assert.rejects(
    async () => createTable(ddb, model, { waitForActive: false }),
    (err) => {
      assert.ok(err instanceof TheorydbError);
      assert.equal(err.code, 'ErrInvalidModel');
      return true;
    },
  );
});

void test('createTable requires throughput when billingMode=PROVISIONED', async () => {
  const model = defineModel(baseSchema);
  const ddb = { send: async () => ({}) } as unknown as DynamoDBClient;

  await assert.rejects(
    async () =>
      createTable(ddb, model, {
        billingMode: 'PROVISIONED',
        waitForActive: false,
      }),
    (err) => {
      assert.ok(err instanceof TheorydbError);
      assert.equal(err.code, 'ErrInvalidOperator');
      return true;
    },
  );
});

void test('createTable syncs TTL when the model declares a ttl role', async () => {
  const model = defineModel(ttlSchema);
  const calls: unknown[] = [];
  const ddb = {
    send: async (cmd: unknown) => {
      calls.push(cmd);
      if (cmd instanceof DescribeTableCommand) {
        return { Table: { TableStatus: 'ACTIVE' } };
      }
      return {};
    },
  } as unknown as DynamoDBClient;

  await createTable(ddb, model, { waitForActive: false });

  assert.equal(calls.length, 3);
  assert.ok(calls[0] instanceof CreateTableCommand);
  assert.ok(calls[1] instanceof DescribeTableCommand);
  assert.ok(calls[2] instanceof UpdateTimeToLiveCommand);
  assert.deepEqual((calls[2] as UpdateTimeToLiveCommand).input, {
    TableName: 'users_contract',
    TimeToLiveSpecification: {
      AttributeName: 'expiresAt',
      Enabled: true,
    },
  });
});

void test('ensureTable syncs TTL for an existing table', async () => {
  const model = defineModel(ttlSchema);
  const calls: unknown[] = [];
  const ddb = {
    send: async (cmd: unknown) => {
      calls.push(cmd);
      if (cmd instanceof DescribeTableCommand) {
        return { Table: { TableStatus: 'ACTIVE' } };
      }
      return {};
    },
  } as unknown as DynamoDBClient;

  await ensureTable(ddb, model, { waitForActive: false });

  assert.equal(calls.length, 3);
  assert.ok(calls[0] instanceof DescribeTableCommand);
  assert.ok(calls[1] instanceof DescribeTableCommand);
  assert.ok(calls[2] instanceof UpdateTimeToLiveCommand);
});
