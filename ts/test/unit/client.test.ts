import assert from 'node:assert/strict';

import {
  BatchGetItemCommand,
  BatchWriteItemCommand,
  ConditionalCheckFailedException,
  DeleteItemCommand,
  GetItemCommand,
  PutItemCommand,
  QueryCommand,
  TransactionCanceledException,
  TransactWriteItemsCommand,
  UpdateItemCommand,
  type DynamoDBClient,
} from '@aws-sdk/client-dynamodb';

import { TheorydbClient } from '../../src/client.js';
import { TheorydbError } from '../../src/errors.js';
import { defineModel } from '../../src/model.js';
import type { TransactAction } from '../../src/transaction.js';

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
    { attribute: 'nickname', type: 'S', optional: true, omit_empty: true },
    { attribute: 'createdAt', type: 'S', roles: ['created_at'] },
    { attribute: 'updatedAt', type: 'S', roles: ['updated_at'] },
    { attribute: 'version', type: 'N', roles: ['version'] },
  ],
});

class StubDdb {
  sent: unknown[] = [];
  calls = 0;

  constructor(
    private readonly handler: (cmd: unknown, call: number) => unknown,
  ) {}

  async send(cmd: unknown): Promise<unknown> {
    this.calls += 1;
    this.sent.push(cmd);
    return this.handler(cmd, this.calls);
  }
}

{
  const ddb = new StubDdb(() => ({}));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );

  await client.create('User', { PK: 'A', SK: 'B' }, { ifNotExists: true });
  const cmd = ddb.sent[0];
  assert.ok(cmd instanceof PutItemCommand);
  assert.equal(cmd.input.TableName, 'users_contract');
  assert.equal(cmd.input.ConditionExpression, 'attribute_not_exists(#pk)');
  assert.deepEqual(cmd.input.ExpressionAttributeNames, { '#pk': 'PK' });
}

{
  const err = new ConditionalCheckFailedException({
    $metadata: {},
    message: 'no',
  });
  const ddb = new StubDdb(() => {
    throw err;
  });
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  await assert.rejects(
    () => client.create('User', { PK: 'A', SK: 'B' }, { ifNotExists: true }),
    (e) => e instanceof TheorydbError && e.code === 'ErrConditionFailed',
  );
}

{
  const ddb = new StubDdb((cmd) => {
    if (cmd instanceof GetItemCommand) {
      return { Item: { PK: { S: 'A' }, SK: { S: 'B' }, version: { N: '1' } } };
    }
    throw new Error('unexpected');
  });
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  const got = await client.get('User', { PK: 'A', SK: 'B' });
  assert.equal(got.PK, 'A');
  assert.equal(got.version, 1);
}

{
  const ddb = new StubDdb((cmd) => {
    if (cmd instanceof GetItemCommand) return {};
    throw new Error('unexpected');
  });
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  await assert.rejects(
    () => client.get('User', { PK: 'A', SK: 'B' }),
    (e) => e instanceof TheorydbError && e.code === 'ErrItemNotFound',
  );
}

{
  const ddb = new StubDdb((cmd) => {
    if (cmd instanceof UpdateItemCommand) return {};
    throw new Error('unexpected');
  });
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );

  await assert.rejects(
    () =>
      client.update('User', { PK: 'A', SK: 'B', nickname: 'x' }, ['nickname']),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidModel',
  );

  await assert.rejects(
    () => client.update('User', { PK: 'A', SK: 'B', version: 0 }, ['PK']),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidModel',
  );

  await client.update('User', { PK: 'A', SK: 'B', nickname: '', version: 0 }, [
    'nickname',
  ]);
  const cmd = ddb.sent[0];
  assert.ok(cmd instanceof UpdateItemCommand);
  assert.ok(cmd.input.UpdateExpression?.includes('REMOVE'));
  assert.ok(cmd.input.UpdateExpression?.includes('ADD #ver :inc'));
}

{
  const ddb = new StubDdb((cmd) => {
    if (cmd instanceof DeleteItemCommand) return {};
    throw new Error('unexpected');
  });
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  await client.delete('User', { PK: 'A', SK: 'B' });
  assert.ok(ddb.sent[0] instanceof DeleteItemCommand);
}

{
  const ddb = new StubDdb((cmd, call) => {
    if (cmd instanceof BatchGetItemCommand) {
      if (call === 1) {
        const pending = cmd.input.RequestItems?.users_contract?.Keys ?? [];
        return {
          Responses: {
            users_contract: [
              { PK: { S: 'A' }, SK: { S: '1' }, version: { N: '0' } },
            ],
          },
          UnprocessedKeys: { users_contract: { Keys: pending.slice(1) } },
        };
      }
      return {
        Responses: {
          users_contract: [
            { PK: { S: 'A' }, SK: { S: '2' }, version: { N: '0' } },
          ],
        },
        UnprocessedKeys: {},
      };
    }
    throw new Error('unexpected');
  });
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  const res = await client.batchGet(
    'User',
    [
      { PK: 'A', SK: '1' },
      { PK: 'A', SK: '2' },
    ],
    {
      maxAttempts: 2,
      baseDelayMs: 0,
    },
  );
  assert.equal(res.items.length, 2);
  assert.equal(res.unprocessedKeys.length, 0);
}

{
  const ddb = new StubDdb((cmd, call) => {
    if (cmd instanceof BatchWriteItemCommand) {
      if (call === 1) {
        return { UnprocessedItems: cmd.input.RequestItems };
      }
      return { UnprocessedItems: {} };
    }
    throw new Error('unexpected');
  });
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  const res = await client.batchWrite(
    'User',
    { puts: [{ PK: 'A', SK: '1' }], deletes: [{ PK: 'A', SK: '2' }] },
    { maxAttempts: 2, baseDelayMs: 0 },
  );
  assert.equal(res.unprocessed.length, 0);
}

{
  const tce = new TransactionCanceledException({ $metadata: {}, message: 'x' });
  const ddb = new StubDdb(() => {
    throw tce;
  });
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  await assert.rejects(
    () =>
      client.transactWrite([
        { kind: 'put', model: 'User', item: { PK: 'A', SK: '1' } },
      ]),
    (e) => e instanceof TheorydbError && e.code === 'ErrConditionFailed',
  );
}

{
  const ddb = new StubDdb((cmd) => {
    if (cmd instanceof TransactWriteItemsCommand) return {};
    throw new Error('unexpected');
  });
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  await client.transactWrite([
    {
      kind: 'put',
      model: 'User',
      item: { PK: 'A', SK: '1' },
      ifNotExists: true,
    },
    { kind: 'delete', model: 'User', key: { PK: 'A', SK: '1' } },
    {
      kind: 'condition',
      model: 'User',
      key: { PK: 'A', SK: '1' },
      conditionExpression: 'attribute_exists(PK)',
    },
  ]);
  assert.ok(ddb.sent[0] instanceof TransactWriteItemsCommand);
}

{
  const ddb = new StubDdb((cmd) => {
    if (cmd instanceof QueryCommand) return { Items: [] };
    throw new Error('unexpected');
  });
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  const qb = client.query('User');
  const page = await qb.partitionKey('A').page();
  assert.deepEqual(page.items, []);
}

{
  const ddb = new StubDdb(() => ({}));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  await assert.rejects(
    () =>
      client.transactWrite([
        { kind: 'wat', model: 'User' } as unknown as TransactAction,
      ]),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
}

{
  const ddb = new StubDdb(() => ({}));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient);
  await assert.rejects(
    () => client.create('Nope', {}),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidModel',
  );
}
