import test from 'node:test';
import assert from 'node:assert/strict';

import { UpdateItemCommand } from '@aws-sdk/client-dynamodb';

import { TheorydbClient } from '../../../../ts/src/client.js';
import { defineModel } from '../../../../ts/src/model.js';
import { createMockDynamoDBClient } from '../../../../ts/src/testkit/index.js';

test('update builder builds safe update/condition expressions', async () => {
  const mock = createMockDynamoDBClient();

  mock.when(UpdateItemCommand, async (cmd) => {
    assert.equal(
      cmd.input.UpdateExpression,
      'SET #u1 = :u1 ADD #u2 :u2',
    );
    assert.equal(cmd.input.ConditionExpression, '#c1 = :c1');
    assert.deepEqual(cmd.input.ExpressionAttributeNames, {
      '#u1': 'name',
      '#u2': 'version',
      '#c1': 'name',
    });
    assert.deepEqual(cmd.input.ExpressionAttributeValues, {
      ':u1': { S: 'v1' },
      ':u2': { N: '1' },
      ':c1': { S: 'v0' },
    });
    assert.equal(cmd.input.ReturnValues, 'NONE');
    return { $metadata: {} };
  });

  const model = defineModel({
    name: 'UpdateContract',
    table: { name: 't' },
    keys: {
      partition: { attribute: 'PK', type: 'S' },
      sort: { attribute: 'SK', type: 'S' },
    },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'SK', type: 'S', roles: ['sk'] },
      { attribute: 'name', type: 'S', optional: true },
      { attribute: 'version', type: 'N', roles: ['version'] },
    ],
  });

  const client = new TheorydbClient(mock.client).register(model);
  await client
    .updateBuilder('UpdateContract', { PK: 'A', SK: 'B' })
    .set('name', 'v1')
    .add('version', 1)
    .condition('name', '=', 'v0')
    .execute();
});

