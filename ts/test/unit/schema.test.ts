import assert from 'node:assert/strict';
import test from 'node:test';

import {
  CreateTableCommand,
  DeleteTableCommand,
  DescribeTableCommand,
  DescribeTimeToLiveCommand,
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
  TTLAttributeMismatchError,
  updateTimeToLive,
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

void test('ensureTable leaves TTL untouched on an existing table', async () => {
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

  assert.equal(calls.length, 1, 'only the existence check may run');
  assert.ok(calls[0] instanceof DescribeTableCommand);
  assert.equal(
    calls.filter(
      (cmd) =>
        cmd instanceof UpdateTimeToLiveCommand ||
        cmd instanceof DescribeTimeToLiveCommand,
    ).length,
    0,
    'ensureTable must not make TTL calls for an existing table',
  );
});

void test('ensureTable still waits on an existing table when requested', async () => {
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

  await ensureTable(ddb, model);

  assert.equal(calls.length, 2);
  assert.ok(calls[0] instanceof DescribeTableCommand);
  assert.ok(calls[1] instanceof DescribeTableCommand);
  assert.equal(
    calls.filter(
      (cmd) =>
        cmd instanceof UpdateTimeToLiveCommand ||
        cmd instanceof DescribeTimeToLiveCommand,
    ).length,
    0,
  );
});

void test('createTable treats an already-enabled TTL on the same attribute as ok', async () => {
  const model = defineModel(ttlSchema);
  const calls: unknown[] = [];
  const ddb = {
    send: async (cmd: unknown) => {
      calls.push(cmd);
      if (cmd instanceof CreateTableCommand) {
        throw { name: 'ResourceInUseException' };
      }
      if (cmd instanceof UpdateTimeToLiveCommand) {
        throw {
          name: 'ValidationException',
          message: 'TimeToLive is already enabled',
        };
      }
      if (cmd instanceof DescribeTimeToLiveCommand) {
        return {
          TimeToLiveDescription: {
            TimeToLiveStatus: 'ENABLED',
            AttributeName: 'expiresAt',
          },
        };
      }
      if (cmd instanceof DescribeTableCommand) {
        return { Table: { TableStatus: 'ACTIVE' } };
      }
      return {};
    },
  } as unknown as DynamoDBClient;

  await createTable(ddb, model, { waitForActive: false });

  assert.equal(
    calls.filter((cmd) => cmd instanceof DescribeTimeToLiveCommand).length,
    1,
  );
});

void test('createTable rejects TTL enabled on a different attribute with a clear error', async () => {
  const model = defineModel(ttlSchema);
  const ddb = {
    send: async (cmd: unknown) => {
      if (cmd instanceof CreateTableCommand) {
        throw { name: 'ResourceInUseException' };
      }
      if (cmd instanceof UpdateTimeToLiveCommand) {
        throw {
          name: 'ValidationException',
          message: 'TimeToLive is active on a different AttributeName',
        };
      }
      if (cmd instanceof DescribeTimeToLiveCommand) {
        return {
          TimeToLiveDescription: {
            TimeToLiveStatus: 'ENABLED',
            AttributeName: 'legacyTtl',
          },
        };
      }
      if (cmd instanceof DescribeTableCommand) {
        return { Table: { TableStatus: 'ACTIVE' } };
      }
      return {};
    },
  } as unknown as DynamoDBClient;

  await assert.rejects(
    async () => createTable(ddb, model, { waitForActive: false }),
    (err) => {
      assert.ok(err instanceof TTLAttributeMismatchError);
      assert.ok(err instanceof TheorydbError);
      assert.equal(err.code, 'ErrInvalidModel');
      assert.match(err.message, /expiresAt/);
      assert.match(err.message, /legacyTtl/);
      assert.match(err.message, /different attribute/);
      return true;
    },
  );
});

void test('updateTimeToLive surfaces the original error when the TTL state cannot be described', async () => {
  const model = defineModel(ttlSchema);
  const original = {
    name: 'ValidationException',
    message: 'TimeToLive is already enabled',
  };
  const ddb = {
    send: async (cmd: unknown) => {
      if (cmd instanceof UpdateTimeToLiveCommand) throw original;
      if (cmd instanceof DescribeTimeToLiveCommand) {
        throw { name: 'InternalServerError' };
      }
      if (cmd instanceof DescribeTableCommand) {
        return { Table: { TableStatus: 'ACTIVE' } };
      }
      return {};
    },
  } as unknown as DynamoDBClient;

  await assert.rejects(
    async () => createTable(ddb, model, { waitForActive: false }),
    (err) => err === original,
  );
});

void test('updateTimeToLive does not describe state for a disable request', async () => {
  const model = defineModel(ttlSchema);
  const calls: unknown[] = [];
  const ddb = {
    send: async (cmd: unknown) => {
      calls.push(cmd);
      if (cmd instanceof UpdateTimeToLiveCommand) {
        throw {
          name: 'ValidationException',
          message: 'TimeToLive is not enabled',
        };
      }
      return {};
    },
  } as unknown as DynamoDBClient;

  await assert.rejects(
    async () => updateTimeToLive(ddb, model, { enabled: false }),
    (err) => (err as { name?: string }).name === 'ValidationException',
  );
  assert.equal(
    calls.filter((cmd) => cmd instanceof DescribeTimeToLiveCommand).length,
    0,
  );
});
