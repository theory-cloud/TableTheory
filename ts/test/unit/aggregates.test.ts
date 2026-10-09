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
  assert.deepEqual(items[0]?.version, '1');
  assert.deepEqual(items[1]?.version, '2');
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
  assert.deepEqual(items[0]?.version, '1');
  assert.deepEqual(items[1]?.version, '2');
}

{
  const items = [{ a: '2' }, { a: '10' }];

  assert.equal(minField(items, 'a'), '2');
  assert.equal(maxField(items, 'a'), '10');
  assert.equal(sumField(items, 'a'), 12);
  assert.equal(averageField(items, 'a'), 6);

  const agg = aggregateField(items, 'a');
  assert.equal(agg.min, '2');
  assert.equal(agg.max, '10');
  assert.equal(agg.sum, 12);
  assert.equal(agg.average, 6);
}

{
  const items = [
    { g: 'a', n: '2' },
    { g: 'a', n: '10' },
    { g: 'b', n: '1' },
  ];

  const results = await new GroupByQuery(async () => items, 'g')
    .count('cnt')
    .sum('n', 'sum')
    .avg('n', 'avg')
    .min('n', 'min')
    .max('n', 'max')
    .having('sum', '>', '11')
    .having('sum', '=', 12)
    .execute();

  assert.equal(results.length, 1);
  assert.equal(results[0]?.key, 'a');
  assert.equal(results[0]?.count, 2);
  assert.equal(results[0]?.aggregates.sum?.sum, 12);
  assert.equal(results[0]?.aggregates.avg?.average, 6);
  assert.equal(results[0]?.aggregates.min?.min, '2');
  assert.equal(results[0]?.aggregates.max?.max, '10');
}

{
  const items = [{ a: '9007199254740993' }, { a: '1' }];

  assert.throws(
    () => sumField(items, 'a'),
    (err) => {
      assert.ok(err instanceof TheorydbError);
      assert.equal(err.code, 'ErrNumberPrecisionLoss');
      return true;
    },
  );
  assert.throws(
    () => averageField(items, 'a'),
    (err) => {
      assert.ok(err instanceof TheorydbError);
      assert.equal(err.code, 'ErrNumberPrecisionLoss');
      return true;
    },
  );

  assert.equal(minField(items, 'a'), '1');
  assert.equal(maxField(items, 'a'), '9007199254740993');
}

function isPrecisionLoss(err: unknown): boolean {
  assert.ok(err instanceof TheorydbError);
  assert.equal(err.code, 'ErrNumberPrecisionLoss');
  return true;
}

{
  const hostile = [
    '1e2000000000',
    '1e100000000',
    '1e999999999',
    '-1e2000000000',
    '+1e-999999999',
  ];

  for (const value of hostile) {
    assert.throws(
      () => minField([{ a: value }, { a: '5' }], 'a'),
      isPrecisionLoss,
    );
    assert.throws(
      () => maxField([{ a: value }, { a: '5' }], 'a'),
      isPrecisionLoss,
    );
    assert.throws(() => sumField([{ a: value }], 'a'), isPrecisionLoss);
    assert.throws(() => averageField([{ a: value }], 'a'), isPrecisionLoss);
    assert.throws(() => aggregateField([{ a: value }], 'a'), isPrecisionLoss);
  }
}

{
  assert.equal(minField([{ a: '1e125' }, { a: '1' }], 'a'), '1');
  assert.equal(maxField([{ a: '1e125' }, { a: '1' }], 'a'), '1e125');
  assert.equal(maxField([{ a: '1e+125' }, { a: '1' }], 'a'), '1e+125');
  assert.equal(minField([{ a: '1e-130' }, { a: '1e-129' }], 'a'), '1e-130');

  for (const value of ['1e126', '1e-131', '-1e126', '+1e-131']) {
    assert.throws(
      () => minField([{ a: value }, { a: '1' }], 'a'),
      isPrecisionLoss,
    );
    assert.throws(
      () => maxField([{ a: value }, { a: '1' }], 'a'),
      isPrecisionLoss,
    );
    assert.throws(() => sumField([{ a: value }], 'a'), isPrecisionLoss);
    assert.throws(() => averageField([{ a: value }], 'a'), isPrecisionLoss);
  }

  assert.throws(() => sumField([{ a: '1e125' }], 'a'), isPrecisionLoss);
  assert.throws(() => averageField([{ a: '1e-130' }], 'a'), isPrecisionLoss);
}

{
  const decimal = [{ a: '0.1' }, { a: '0.2' }];

  assert.throws(() => sumField(decimal, 'a'), isPrecisionLoss);
  assert.throws(() => averageField(decimal, 'a'), isPrecisionLoss);
  assert.throws(() => aggregateField(decimal, 'a'), isPrecisionLoss);

  assert.equal(sumField([{ a: '0.5' }, { a: '0.25' }], 'a'), 0.75);
  assert.equal(
    sumField([{ a: '0.5' }, { a: '0.25' }, { a: '0.625' }], 'a'),
    1.375,
  );
  assert.equal(averageField([{ a: '0.5' }, { a: '0.25' }], 'a'), 0.375);
  assert.equal(averageField([{ a: '0.5' }, { a: '1.5' }], 'a'), 1);
  assert.equal(sumField([{ a: '-0.5' }, { a: '2.5e-1' }], 'a'), -0.25);
  assert.equal(sumField([{ a: '1e3' }, { a: '2.5e2' }], 'a'), 1250);
  assert.equal(averageField([{ a: '1e3' }, { a: '2.5e2' }], 'a'), 625);

  const carry = [
    { a: '10000000000000000' },
    { a: '1' },
    { a: '-10000000000000000' },
  ];
  assert.equal(sumField(carry, 'a'), 1);

  assert.throws(
    () => sumField([{ a: '9007199254740992' }, { a: '1' }], 'a'),
    isPrecisionLoss,
  );
  assert.equal(averageField([{ a: '1' }, { a: '2' }], 'a'), 1.5);
  assert.throws(
    () => averageField([{ a: '1' }, { a: '2' }, { a: '2' }], 'a'),
    isPrecisionLoss,
  );

  const aggregate = aggregateField([{ a: '0.5' }, { a: '0.375' }], 'a');
  assert.equal(aggregate.sum, 0.875);
  assert.equal(aggregate.average, 0.4375);
}

{
  assert.equal(
    minField([{ a: '9007199254740993' }, { a: '9007199254740992' }], 'a'),
    '9007199254740992',
  );
  assert.equal(
    maxField([{ a: '9007199254740993' }, { a: '9007199254740992' }], 'a'),
    '9007199254740993',
  );
  assert.equal(minField([{ a: '0.1' }, { a: '0.2' }], 'a'), '0.1');
  assert.equal(maxField([{ a: '0.1' }, { a: '0.2' }], 'a'), '0.2');
}

{
  const items = [
    { g: 'a', n: '0.5' },
    { g: 'a', n: '0.25' },
    { g: 'b', n: '1' },
  ];

  const results = await new GroupByQuery(async () => items, 'g')
    .count('cnt')
    .sum('n', 'sum')
    .avg('n', 'avg')
    .min('n', 'min')
    .max('n', 'max')
    .having('sum', '=', '0.75')
    .execute();

  assert.equal(results.length, 1);
  assert.equal(results[0]?.key, 'a');
  assert.equal(results[0]?.aggregates.sum?.sum, 0.75);
  assert.equal(results[0]?.aggregates.avg?.average, 0.375);
  assert.equal(results[0]?.aggregates.min?.min, '0.25');
  assert.equal(results[0]?.aggregates.max?.max, '0.5');

  await assert.rejects(
    new GroupByQuery(async () => [{ g: 'a', n: '1e2000000000' }], 'g')
      .sum('n', 'sum')
      .execute(),
    isPrecisionLoss,
  );
  await assert.rejects(
    new GroupByQuery(async () => [{ g: 'a', n: '1e2000000000' }], 'g')
      .min('n', 'min')
      .execute(),
    isPrecisionLoss,
  );
  await assert.rejects(
    new GroupByQuery(
      async () => [
        { g: 'a', n: '1' },
        { g: 'a', n: '2' },
      ],
      'g',
    )
      .sum('n', 'sum')
      .having('sum', '=', '0.1')
      .execute(),
    isPrecisionLoss,
  );
}
