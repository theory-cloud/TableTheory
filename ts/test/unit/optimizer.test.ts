import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { DynamoDBClient } from '@aws-sdk/client-dynamodb';

import { TheorydbClient } from '../../src/client.js';
import { defineModel } from '../../src/model.js';
import {
  analyzeConditions,
  QueryOptimizer,
  selectOptimalIndex,
} from '../../src/optimizer.js';

class StubDdb {
  async send(): Promise<unknown> {
    return { Items: [] };
  }
}

const User = defineModel({
  name: 'UserOpt',
  table: { name: 'users_contract' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
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
});

void test('optimizer reports missing query partition key deterministically', () => {
  const client = new TheorydbClient(new StubDdb() as unknown as DynamoDBClient)
    .register(User)
    .withSendOptions(undefined);

  const optimizer = new QueryOptimizer();
  const builder = client.query('UserOpt');
  const plan = optimizer.explain(builder.describe());

  assert.equal(plan.operation, 'Query');
  assert.ok(plan.optimizationHints.some((h) => h.includes('partitionKey()')));

  const plan2 = optimizer.explain(builder.describe());
  assert.equal(plan.id, plan2.id);
});

void test('optimizer preserves scan warnings when no indexable condition exists', () => {
  const client = new TheorydbClient(new StubDdb() as unknown as DynamoDBClient)
    .register(User)
    .withSendOptions(undefined);

  const optimizer = new QueryOptimizer({ maxParallelism: 3 });
  const scan = client.scan('UserOpt').filter('emailHash', 'CONTAINS', 'x');
  const plan = optimizer.explain(scan.describe());

  assert.equal(plan.operation, 'Scan');
  assert.ok(plan.optimizationHints.some((h) => h.startsWith('WARNING: Scan')));
  assert.ok(plan.optimizationHints.some((h) => h.includes('Filters')));
  assert.equal(plan.parallelSegments, 3);
  assert.ok(
    plan.optimizationHints.some((h) => h.includes('scanAllSegments(3)')),
  );
});

void test('optimizer omits projection hint when projection is present', () => {
  const client = new TheorydbClient(new StubDdb() as unknown as DynamoDBClient)
    .register(User)
    .withSendOptions(undefined);

  const optimizer = new QueryOptimizer();
  const query = client
    .query('UserOpt')
    .partitionKey('A')
    .projection(['PK', 'SK']);
  const plan = optimizer.explain(query.describe());

  assert.ok(!plan.optimizationHints.some((h) => h.includes('projection()')));
});

void test('optimizer selects indexes using Go-compatible condition analysis', () => {
  const required = analyzeConditions([
    { field: 'createdAt', operator: 'BETWEEN' },
    { field: 'emailHash', operator: 'EQ' },
  ]);

  assert.deepEqual(required, {
    partitionKey: 'emailHash',
    sortKey: 'createdAt',
    sortKeyOp: 'between',
  });

  const selected = selectOptimalIndex(required, [
    {
      type: 'PRIMARY',
      partition: 'PK',
      sort: 'SK',
      projectionType: 'ALL',
    },
    {
      name: 'gsi-email',
      type: 'GSI',
      partition: 'emailHash',
      projectionType: 'ALL',
    },
  ]);

  assert.equal(selected?.name, 'gsi-email');
});

void test('optimizer explain selects GSI from query filter conditions', () => {
  const client = new TheorydbClient(new StubDdb() as unknown as DynamoDBClient)
    .register(User)
    .withSendOptions(undefined);

  const optimizer = new QueryOptimizer();
  const query = client.query('UserOpt').filter('emailHash', '=', 'x');
  const plan = optimizer.explain(query.describe());

  assert.equal(plan.operation, 'Query');
  assert.equal(plan.indexName, 'gsi-email');
  assert.ok(
    plan.optimizationHints.some((h) => h.includes('selected GSI gsi-email')),
  );
});

void test('optimizer explain keeps primary index when partition key is set', () => {
  const client = new TheorydbClient(new StubDdb() as unknown as DynamoDBClient)
    .register(User)
    .withSendOptions(undefined);

  const optimizer = new QueryOptimizer();
  const query = client
    .query('UserOpt')
    .partitionKey('USER#1')
    .filter('emailHash', '=', 'x');
  const plan = optimizer.explain(query.describe());

  assert.equal(plan.operation, 'Query');
  assert.equal(plan.indexName, undefined);
  assert.ok(
    plan.optimizationHints.some((h) =>
      h.includes('selected the table primary'),
    ),
  );
});
