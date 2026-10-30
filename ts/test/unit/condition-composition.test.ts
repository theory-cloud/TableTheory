import assert from 'node:assert/strict';
import test from 'node:test';

import type { AttributeValue } from '@aws-sdk/client-dynamodb';

import { TheorydbClient } from '../../src/client.js';
import { TheorydbError } from '../../src/errors.js';
import { defineModel } from '../../src/model.js';
import { createStatefulDynamoDBClient } from '../../src/testkit/index.js';

const TABLE_NAME = 'composed_conditions';

const Versioned = defineModel({
  name: 'Versioned',
  table: { name: TABLE_NAME },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'name', type: 'S' },
    { attribute: 'version', type: 'N', roles: ['version'] },
  ],
});

function seed(
  fake: ReturnType<typeof createStatefulDynamoDBClient>['fake'],
  name: string,
): void {
  fake.seed(TABLE_NAME, {
    PK: { S: 'A' },
    SK: { S: '1' },
    name: { S: name },
    version: { N: '3' },
  });
}

// The update expression takes `#c0`/`:c0`, so the generated version condition
// owns `#c1`/`:c1`. A caller condition that reuses those, and also declares
// `#caller1`/`:caller1`, is the chained collision this test is for: the rename of
// `#c1` must not land on the caller's own `#caller1` placeholder, because then
// both caller predicates would point at one attribute and the one that was
// overwritten would stop being checked.
function chainedCollisionAction(nameValue: string) {
  const values: Record<string, AttributeValue> = {
    ':c1': { S: nameValue },
    ':caller1': { N: '3' },
  };
  return {
    kind: 'update' as const,
    model: 'Versioned',
    item: { PK: 'A', SK: '1', name: nameValue },
    fields: ['name'] as const,
    expectedVersion: 3,
    conditionExpression: '#c1 = :c1 AND #caller1 = :caller1',
    expressionAttributeNames: { '#c1': 'name', '#caller1': 'version' },
    expressionAttributeValues: values,
  };
}

void test('a chained condition collision leaves every caller predicate checked', async () => {
  const { client, fake } = createStatefulDynamoDBClient();
  const theorydb = new TheorydbClient(client).register(Versioned);

  // Both caller predicates hold: the name matches and the version matches.
  seed(fake, 'alice');
  await theorydb.transactWrite([chainedCollisionAction('alice')]);
  assert.equal(fake.items(TABLE_NAME)[0]?.['name']?.['S'], 'alice');

  // The name predicate is false while the version predicate is true. A
  // composition that collapsed the two caller placeholders would drop the name
  // check and let this write through.
  seed(fake, 'bob');
  await assert.rejects(
    theorydb.transactWrite([chainedCollisionAction('alice')]),
    (err: unknown) =>
      err instanceof TheorydbError &&
      (err.code === 'ErrConditionFailed' ||
        err.code === 'ErrTransactionFailed' ||
        err.code === 'ErrTransactionConflict'),
  );
  assert.equal(
    fake.items(TABLE_NAME)[0]?.['name']?.['S'],
    'bob',
    'the rejected write must not have changed the stored item',
  );
});

void test('a caller condition with no collision keeps its own placeholders', async () => {
  const { client, fake } = createStatefulDynamoDBClient();
  const theorydb = new TheorydbClient(client).register(Versioned);

  seed(fake, 'alice');
  await theorydb.transactWrite([
    {
      kind: 'update',
      model: 'Versioned',
      item: { PK: 'A', SK: '1', name: 'alice' },
      fields: ['name'],
      expectedVersion: 3,
      conditionExpression: '#mine = :mine',
      expressionAttributeNames: { '#mine': 'name' },
      expressionAttributeValues: { ':mine': { S: 'alice' } },
    },
  ]);
  assert.equal(fake.items(TABLE_NAME)[0]?.['name']?.['S'], 'alice');

  seed(fake, 'bob');
  await assert.rejects(
    theorydb.transactWrite([
      {
        kind: 'update',
        model: 'Versioned',
        item: { PK: 'A', SK: '1', name: 'alice' },
        fields: ['name'],
        expectedVersion: 3,
        conditionExpression: '#mine = :mine',
        expressionAttributeNames: { '#mine': 'name' },
        expressionAttributeValues: { ':mine': { S: 'alice' } },
      },
    ]),
    (err: unknown) => err instanceof TheorydbError,
  );
});
