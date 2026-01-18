import assert from 'node:assert/strict';

import {
  QueryCommand,
  ScanCommand,
  type DynamoDBClient,
} from '@aws-sdk/client-dynamodb';

import { encodeCursor } from '../../src/cursor.js';
import { TheorydbClient } from '../../src/client.js';
import { TheorydbError } from '../../src/errors.js';
import { defineModel } from '../../src/model.js';

class StubDdb {
  calls = 0;
  last: unknown | undefined;
  constructor(
    private readonly handler: (cmd: unknown, call: number) => unknown,
  ) {}
  async send(cmd: unknown): Promise<unknown> {
    this.calls += 1;
    this.last = cmd;
    return this.handler(cmd, this.calls);
  }
}

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
    { attribute: 'createdAt', type: 'S', roles: ['created_at'] },
    { attribute: 'updatedAt', type: 'S', roles: ['updated_at'] },
    { attribute: 'version', type: 'N', roles: ['version'] },
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

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  await assert.rejects(
    () => client.query('User').page(),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
}

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  await assert.rejects(
    () =>
      client
        .query('User')
        .usingIndex('gsi-email')
        .consistentRead()
        .partitionKey('x')
        .page(),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
}

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  await assert.rejects(
    () => client.query('User').partitionKey('A').sortKey('between', '1').page(),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
}

{
  const ddb = new StubDdb((cmd) => {
    if (cmd instanceof QueryCommand) {
      return {
        Items: [{ PK: { S: 'A' }, SK: { S: '1' }, version: { N: '0' } }],
        LastEvaluatedKey: { PK: { S: 'A' }, SK: { S: '1' } },
      };
    }
    throw new Error('unexpected');
  });
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  const page = await client.query('User').partitionKey('A').limit(1).page();
  assert.equal(page.items.length, 1);
  assert.ok(page.cursor);
  assert.ok(ddb.last instanceof QueryCommand);
  assert.equal(ddb.last.input.ScanIndexForward, true);
}

{
  const cursor = encodeCursor({
    lastKey: { PK: { S: 'A' }, SK: { S: '1' } },
    index: 'gsi-email',
    sort: 'ASC',
  });
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  await assert.rejects(
    () => client.query('User').partitionKey('A').cursor(cursor).page(),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
}

{
  const ddb = new StubDdb((cmd) => {
    if (cmd instanceof ScanCommand) {
      return {
        Items: [],
        LastEvaluatedKey: { PK: { S: 'A' }, SK: { S: '1' } },
      };
    }
    throw new Error('unexpected');
  });
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  const page = await client.scan('User').limit(1).page();
  assert.equal(page.items.length, 0);
  assert.ok(page.cursor);
  assert.ok(ddb.last instanceof ScanCommand);
}

{
  const cursor = encodeCursor({
    lastKey: { PK: { S: 'A' }, SK: { S: '1' } },
    index: 'other',
  });
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  await assert.rejects(
    () => client.scan('User').usingIndex('gsi-email').cursor(cursor).page(),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
}

{
  const enc = defineModel({
    name: 'Enc',
    table: { name: 'enc_contract' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'secret', type: 'S', encryption: { v: 1 } },
    ],
  });
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    enc,
  );
  await assert.rejects(
    () => client.query('Enc').partitionKey('A').page(),
    (e) =>
      e instanceof TheorydbError && e.code === 'ErrEncryptionNotConfigured',
  );
}

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );

  await client
    .query('User')
    .partitionKey('A')
    .filter('emailHash', '=', 'hash')
    .page();

  assert.ok(ddb.last instanceof QueryCommand);
  assert.equal(ddb.last.input.FilterExpression, '#f1 = :f1');
  assert.deepEqual(
    ddb.last.input.ExpressionAttributeNames?.['#f1'],
    'emailHash',
  );
  assert.deepEqual(ddb.last.input.ExpressionAttributeValues?.[':f1'], {
    S: 'hash',
  });
}

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );

  await client
    .query('User')
    .partitionKey('A')
    .filter('version', '>', 1)
    .orFilterGroup((f) =>
      f.filter('emailHash', '=', 'a').orFilter('emailHash', '=', 'b'),
    )
    .page();

  assert.ok(ddb.last instanceof QueryCommand);
  assert.equal(
    ddb.last.input.FilterExpression,
    '#f1 > :f1 OR (#f2 = :f2 OR #f2 = :f3)',
  );
}

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  assert.throws(
    () =>
      client
        .query('User')
        .partitionKey('A')
        .filter('emailHash', 'IN', ['a', 1]),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidModel',
  );
}

{
  const enc = defineModel({
    name: 'EncQuery',
    table: { name: 'enc_contract' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'secret', type: 'S', encryption: { v: 1 } },
    ],
  });
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    enc,
  );
  assert.throws(
    () => client.query('EncQuery').partitionKey('A').filter('secret', '=', 'x'),
    (e) =>
      e instanceof TheorydbError && e.code === 'ErrEncryptedFieldNotQueryable',
  );
}

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );

  await client.scan('User').filter('emailHash', 'EXISTS').limit(1).page();

  assert.ok(ddb.last instanceof ScanCommand);
  assert.equal(ddb.last.input.FilterExpression, 'attribute_exists(#f1)');
}

{
  const ddb = new StubDdb((cmd) => {
    if (cmd instanceof ScanCommand) {
      if (cmd.input.Segment === 0) {
        return {
          Items: [{ PK: { S: 'A' }, SK: { S: '0' }, version: { N: '0' } }],
        };
      }
      if (cmd.input.Segment === 1) {
        return {
          Items: [{ PK: { S: 'A' }, SK: { S: '1' }, version: { N: '0' } }],
        };
      }
    }
    throw new Error('unexpected');
  });
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );

  const items = await client
    .scan('User')
    .scanAllSegments(2, { concurrency: 1 });
  assert.equal(items.length, 2);
  assert.deepEqual(
    items.map((i) => i.SK),
    ['0', '1'],
  );
}

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );

  await client.scan('User').parallelScan(0, 2).limit(1).page();

  assert.ok(ddb.last instanceof ScanCommand);
  assert.equal(ddb.last.input.Segment, 0);
  assert.equal(ddb.last.input.TotalSegments, 2);
}

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  assert.throws(
    () => client.scan('User').parallelScan(2, 2),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
}

{
  const ddb = new StubDdb((cmd, call) => {
    if (cmd instanceof QueryCommand) {
      if (call === 1) return { Items: [] };
      return {
        Items: [{ PK: { S: 'A' }, SK: { S: '1' }, version: { N: '0' } }],
      };
    }
    throw new Error('unexpected');
  });
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  const page = await client
    .query('User')
    .partitionKey('A')
    .limit(1)
    .pageWithRetry({ maxAttempts: 2, baseDelayMs: 0 });
  assert.equal(page.items.length, 1);
  assert.equal(page.items[0]!.SK, '1');
}

{
  const ddb = new StubDdb((cmd, call) => {
    if (cmd instanceof ScanCommand) {
      if (call === 1) return { Items: [] };
      return {
        Items: [{ PK: { S: 'A' }, SK: { S: '1' }, version: { N: '0' } }],
      };
    }
    throw new Error('unexpected');
  });
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  const page = await client
    .scan('User')
    .limit(1)
    .pageWithRetry({ maxAttempts: 2, baseDelayMs: 0 });
  assert.equal(page.items.length, 1);
  assert.equal(page.items[0]!.SK, '1');
}

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  assert.throws(
    () => client.query('User').partitionKey('A').filter('nope', '=', 'x'),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
}

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  assert.throws(
    () =>
      client.query('User').partitionKey('A').filter('emailHash', 'LIKE', 'x'),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
}

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  assert.throws(
    () =>
      client
        .query('User')
        .partitionKey('A')
        .filter('emailHash', 'BETWEEN', 'a'),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
}

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  assert.throws(
    () => client.query('User').partitionKey('A').filter('emailHash', 'IN', 'a'),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
}

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  const big = Array.from({ length: 101 }, () => 'x');
  assert.throws(
    () => client.query('User').partitionKey('A').filter('emailHash', 'IN', big),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
}

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  assert.throws(
    () =>
      client.query('User').partitionKey('A').filter('emailHash', 'EXISTS', 'x'),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
}

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  assert.throws(
    () =>
      client
        .query('User')
        .partitionKey('A')
        .filter('emailHash', 'NOT_EXISTS', 'x'),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
}

{
  const ddb = new StubDdb((cmd) => {
    if (cmd instanceof QueryCommand) return { Items: [] };
    throw new Error('unexpected');
  });
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );

  await client
    .query('User')
    .partitionKey('A')
    .filterGroup(() => {})
    .page();
  assert.ok(ddb.last instanceof QueryCommand);
  assert.equal(ddb.last.input.FilterExpression, undefined);
}

{
  const ddb = new StubDdb((cmd, call) => {
    if (cmd instanceof QueryCommand) {
      if (call === 1) return { Items: [] };
      return {
        Items: [{ PK: { S: 'A' }, SK: { S: '1' }, version: { N: '0' } }],
      };
    }
    throw new Error('unexpected');
  });
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  const page = await client
    .query('User')
    .partitionKey('A')
    .pageWithRetry({
      maxAttempts: 2,
      baseDelayMs: 0,
      verify: (p) => p.items.length > 0,
    });
  assert.equal(page.items.length, 1);
}

{
  const ddb = new StubDdb((cmd) => {
    if (cmd instanceof QueryCommand) return { Items: [] };
    throw new Error('unexpected');
  });
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  const page = await client.query('User').partitionKey('A').pageWithRetry({
    maxAttempts: 5,
    baseDelayMs: 0,
    retryOnEmpty: false,
  });
  assert.equal(page.items.length, 0);
  assert.equal(ddb.calls, 1);
}

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  await assert.rejects(
    () => client.scan('User').scanAllSegments(1, { concurrency: Number.NaN }),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
  assert.equal(ddb.calls, 0);
}

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  await assert.rejects(
    () =>
      client
        .scan('User')
        .usingIndex('gsi-email')
        .consistentRead()
        .scanAllSegments(1),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
}

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  await assert.rejects(
    () => client.query('User').usingIndex('missing').partitionKey('x').page(),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
}
