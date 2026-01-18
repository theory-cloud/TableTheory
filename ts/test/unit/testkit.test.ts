import test from 'node:test';
import assert from 'node:assert/strict';

import { PutItemCommand, UpdateItemCommand } from '@aws-sdk/client-dynamodb';

import { TheorydbClient } from '../../src/client.js';
import { defineModel } from '../../src/model.js';
import {
  createDeterministicEncryptionProvider,
  createMockDynamoDBClient,
  fixedNow,
} from '../../src/testkit/index.js';

function assertInstanceOf<T>(
  value: unknown,
  ctor: new (input: never) => T,
): asserts value is T {
  assert.ok(value instanceof ctor);
}

function assertDefined<T>(value: T): asserts value is NonNullable<T> {
  assert.ok(value !== undefined && value !== null);
}

test('createMockDynamoDBClient is strict by default', async () => {
  const mock = createMockDynamoDBClient();
  await assert.rejects(() =>
    mock.client.send(new PutItemCommand({ TableName: 't', Item: {} })),
  );
});

test('createMockDynamoDBClient matches handlers by command name', async () => {
  const mock = createMockDynamoDBClient();

  const ShadowUpdateItemCommand = class UpdateItemCommand {};
  mock.when(
    ShadowUpdateItemCommand as unknown as typeof UpdateItemCommand,
    async () => ({ $metadata: {} }),
  );

  await mock.client.send(
    new UpdateItemCommand({
      TableName: 't',
      Key: { PK: { S: 'A' } },
      UpdateExpression: 'SET #n = :v',
      ExpressionAttributeNames: { '#n': 'name' },
      ExpressionAttributeValues: { ':v': { S: 'x' } },
    }),
  );
});

test('client now() injection drives createdAt/updatedAt + update :now', async () => {
  const mock = createMockDynamoDBClient();

  mock.when(PutItemCommand, async () => ({ $metadata: {} }));
  mock.when(UpdateItemCommand, async () => ({ $metadata: {} }));

  const model = defineModel({
    name: 'T',
    table: { name: 't' },
    keys: {
      partition: { attribute: 'PK', type: 'S' },
      sort: { attribute: 'SK', type: 'S' },
    },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'SK', type: 'S', roles: ['sk'] },
      { attribute: 'name', type: 'S', optional: true },
      { attribute: 'createdAt', type: 'S', roles: ['created_at'] },
      { attribute: 'updatedAt', type: 'S', roles: ['updated_at'] },
      { attribute: 'version', type: 'N', roles: ['version'] },
    ],
  });

  const now = '2026-01-16T00:00:00.000000000Z';
  const client = new TheorydbClient(mock.client, {
    now: fixedNow(now),
  }).register(model);

  await client.create('T', { PK: 'A', SK: 'B', name: 'v0' });
  const putCall = mock.calls[0];
  assertInstanceOf(putCall, PutItemCommand);

  const putInput = putCall.input;
  assertDefined(putInput.Item);
  assertDefined(putInput.Item.createdAt);
  assertDefined(putInput.Item.updatedAt);
  assertDefined(putInput.Item.version);

  assert.equal(putInput.Item.createdAt.S, now);
  assert.equal(putInput.Item.updatedAt.S, now);
  assert.equal(putInput.Item.version.N, '0');

  await client.update('T', { PK: 'A', SK: 'B', name: 'v1', version: 0 }, [
    'name',
  ]);
  const updateCall = mock.calls[1];
  assertInstanceOf(updateCall, UpdateItemCommand);

  const updInput = updateCall.input;
  assertDefined(updInput.ExpressionAttributeValues);
  assertDefined(updInput.ExpressionAttributeValues[':now']);
  assert.equal(updInput.ExpressionAttributeValues[':now'].S, now);
});

test('createDeterministicEncryptionProvider round-trips + binds AAD to attribute', async () => {
  const provider = createDeterministicEncryptionProvider('seed');

  const env = await provider.encrypt(new TextEncoder().encode('secret'), {
    model: 'T',
    attribute: 'secret',
  });
  const pt = await provider.decrypt(env, { model: 'T', attribute: 'secret' });
  assert.equal(new TextDecoder().decode(pt), 'secret');

  await assert.rejects(() =>
    provider.decrypt(env, { model: 'T', attribute: 'other' }),
  );
});
