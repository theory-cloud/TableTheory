import assert from 'node:assert/strict';

import {
  QueryCommand,
  ScanCommand,
  type DynamoDBClient,
} from '@aws-sdk/client-dynamodb';

import {
  GroupByQuery,
  aggregateField,
  averageField,
  countDistinct,
  maxField,
  minField,
  sumField,
} from '../../src/aggregates.js';
import { TheorydbClient } from '../../src/client.js';
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
  name: 'UserAgg',
  table: { name: 'users_contract' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'version', type: 'N', roles: ['version'] },
  ],
});

{
  const items = [
    { n: 1, k: 'a' },
    { n: 2, k: 'a' },
    { n: 0, k: 0 },
    { k: 'b' },
    {},
  ];

  assert.equal(sumField(items, 'n'), 3);
  assert.equal(averageField(items, 'n'), 1);
  assert.equal(minField(items, 'n'), 1);
  assert.equal(maxField(items, 'n'), 2);

  const agg = aggregateField(items, 'n');
  assert.equal(agg.count, 5);
  assert.equal(agg.sum, 3);
  assert.equal(agg.average, 1);
  assert.equal(agg.min, 1);
  assert.equal(agg.max, 2);

  assert.equal(countDistinct(items, 'k'), 2);
}

{
  const items = [
    { g: 'a', n: 1 },
    { g: 'a', n: 2 },
    { g: 'b', n: 10 },
    { g: '', n: 100 },
    { g: 'a', n: 0 },
    { g: 'a', n: undefined },
  ];

  const results = await new GroupByQuery(async () => items, 'g')
    .count('cnt')
    .sum('n', 'sum')
    .avg('n', 'avg')
    .min('n', 'min')
    .max('n', 'max')
    .having('COUNT(*)', '>', 1)
    .having('sum', '=', 3)
    .execute();

  assert.equal(results.length, 1);
  assert.equal(results[0]?.key, 'a');
  assert.equal(results[0]?.count, 4);
  assert.equal(results[0]?.aggregates.sum?.sum, 3);
  assert.equal(results[0]?.aggregates.avg?.average, 1);
  assert.equal(results[0]?.aggregates.min?.min, 1);
  assert.equal(results[0]?.aggregates.max?.max, 2);
}

{
  const ddb = new StubDdb((cmd, call) => {
    if (!(cmd instanceof QueryCommand)) throw new Error('unexpected');
    if (call === 1) {
      assert.equal(cmd.input.ExclusiveStartKey, undefined);
      return {
        Items: [{ PK: { S: 'A' }, SK: { S: '1' }, version: { N: '1' } }],
        LastEvaluatedKey: { PK: { S: 'A' }, SK: { S: '1' } },
      };
    }
    assert.deepEqual(cmd.input.ExclusiveStartKey, {
      PK: { S: 'A' },
      SK: { S: '1' },
    });
    return {
      Items: [{ PK: { S: 'A' }, SK: { S: '2' }, version: { N: '2' } }],
    };
  });
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );

  const items = await client.query('UserAgg').partitionKey('A').all();
  assert.equal(items.length, 2);
  assert.deepEqual(items[0]?.version, 1);
  assert.deepEqual(items[1]?.version, 2);
}

{
  const ddb = new StubDdb((cmd, call) => {
    if (!(cmd instanceof ScanCommand)) throw new Error('unexpected');
    if (call === 1) {
      assert.equal(cmd.input.ExclusiveStartKey, undefined);
      return {
        Items: [{ PK: { S: 'A' }, SK: { S: '1' }, version: { N: '1' } }],
        LastEvaluatedKey: { PK: { S: 'A' }, SK: { S: '1' } },
      };
    }
    assert.deepEqual(cmd.input.ExclusiveStartKey, {
      PK: { S: 'A' },
      SK: { S: '1' },
    });
    return {
      Items: [{ PK: { S: 'A' }, SK: { S: '2' }, version: { N: '2' } }],
    };
  });
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    User,
  );

  const items = await client.scan('UserAgg').all();
  assert.equal(items.length, 2);
  assert.deepEqual(items[0]?.version, 1);
  assert.deepEqual(items[1]?.version, 2);
}
