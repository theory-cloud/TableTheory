import assert from 'node:assert/strict';

import type { DynamoDBClient } from '@aws-sdk/client-dynamodb';

import { TheorydbClient } from '../../src/client.js';
import { defineModel } from '../../src/model.js';
import { QueryOptimizer } from '../../src/optimizer.js';

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

{
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
}

{
  const client = new TheorydbClient(new StubDdb() as unknown as DynamoDBClient)
    .register(User)
    .withSendOptions(undefined);

  const optimizer = new QueryOptimizer({ maxParallelism: 3 });
  const scan = client.scan('UserOpt').filter('emailHash', '=', 'x');
  const plan = optimizer.explain(scan.describe());

  assert.equal(plan.operation, 'Scan');
  assert.ok(plan.optimizationHints.some((h) => h.startsWith('WARNING: Scan')));
  assert.ok(plan.optimizationHints.some((h) => h.includes('Filters')));
  assert.equal(plan.parallelSegments, 3);
  assert.ok(
    plan.optimizationHints.some((h) => h.includes('scanAllSegments(3)')),
  );
}

{
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
}
