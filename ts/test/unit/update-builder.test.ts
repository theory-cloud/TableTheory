import assert from 'node:assert/strict';

import {
  UpdateItemCommand,
  type AttributeValue,
} from '@aws-sdk/client-dynamodb';

import { TheorydbClient } from '../../src/client.js';
import { encryptAttributeValue } from '../../src/encryption.js';
import { TheorydbError } from '../../src/errors.js';
import { defineModel } from '../../src/model.js';
import {
  createDeterministicEncryptionProvider,
  createMockDynamoDBClient,
} from '../../src/testkit/index.js';

{
  const mock = createMockDynamoDBClient();

  mock.when(UpdateItemCommand, async (cmd) => {
    assert.equal(
      cmd.input.UpdateExpression,
      'SET #u1 = :u1 REMOVE #u2 ADD #u3 :u2 DELETE #u4 :u3',
    );
    assert.deepEqual(cmd.input.ExpressionAttributeNames, {
      '#u1': 'name',
      '#u2': 'nickname',
      '#u3': 'count',
      '#u4': 'tags',
    });
    assert.deepEqual(cmd.input.ExpressionAttributeValues, {
      ':u1': { S: 'v1' },
      ':u2': { N: '1' },
      ':u3': { SS: ['a'] },
    } satisfies Record<string, AttributeValue>);
    assert.equal(cmd.input.ReturnValues, 'NONE');
    return { $metadata: {} };
  });

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
      { attribute: 'nickname', type: 'S', optional: true },
      { attribute: 'count', type: 'N', optional: true },
      { attribute: 'tags', type: 'SS', optional: true },
    ],
  });

  const client = new TheorydbClient(mock.client).register(model);
  await client
    .updateBuilder('T', { PK: 'A', SK: 'B' })
    .set('name', 'v1')
    .remove('nickname')
    .add('count', 1)
    .delete('tags', 'a')
    .execute();
}

{
  const mock = createMockDynamoDBClient();

  mock.when(UpdateItemCommand, async (cmd) => {
    assert.equal(cmd.input.UpdateExpression, 'SET #u1 = list_append(#u1, :u1)');
    assert.deepEqual(cmd.input.ExpressionAttributeNames, { '#u1': 'items' });
    assert.deepEqual(cmd.input.ExpressionAttributeValues, {
      ':u1': { L: [{ S: 'x' }, { N: '1' }] },
    });
    return { $metadata: {} };
  });

  const model = defineModel({
    name: 'L',
    table: { name: 't' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'items', type: 'L', optional: true },
    ],
  });

  const client = new TheorydbClient(mock.client).register(model);
  await client
    .updateBuilder('L', { PK: 'A' })
    .appendToList('items', ['x', 1])
    .execute();
}

{
  const mock = createMockDynamoDBClient();

  mock.when(UpdateItemCommand, async (cmd) => {
    assert.equal(cmd.input.UpdateExpression, 'SET #u1[0] = :u1');
    assert.deepEqual(cmd.input.ExpressionAttributeNames, { '#u1': 'items' });
    assert.deepEqual(cmd.input.ExpressionAttributeValues, {
      ':u1': { M: { a: { N: '1' } } },
    });
    return { $metadata: {} };
  });

  const model = defineModel({
    name: 'L',
    table: { name: 't' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'items', type: 'L', optional: true },
    ],
  });

  const client = new TheorydbClient(mock.client).register(model);
  await client
    .updateBuilder('L', { PK: 'A' })
    .setListElement('items', 0, { a: 1 })
    .execute();
}

{
  const mock = createMockDynamoDBClient();

  mock.when(UpdateItemCommand, async (cmd) => {
    assert.equal(cmd.input.UpdateExpression, 'REMOVE #u1[1]');
    assert.deepEqual(cmd.input.ExpressionAttributeNames, { '#u1': 'items' });
    return { $metadata: {} };
  });

  const model = defineModel({
    name: 'L',
    table: { name: 't' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'items', type: 'L', optional: true },
    ],
  });

  const client = new TheorydbClient(mock.client).register(model);
  await client
    .updateBuilder('L', { PK: 'A' })
    .removeFromListAt('items', 1)
    .execute();
}

{
  const mock = createMockDynamoDBClient();

  mock.when(UpdateItemCommand, async (cmd) => {
    assert.equal(cmd.input.UpdateExpression, 'SET #u1 = :u1');
    assert.equal(
      cmd.input.ConditionExpression,
      '#c1 = :c1 OR #c2 > :c2 AND attribute_exists(#c3) AND attribute_not_exists(#c4) AND #c5 = :c3',
    );
    assert.deepEqual(cmd.input.ExpressionAttributeNames, {
      '#u1': 'name',
      '#c1': 'name',
      '#c2': 'count',
      '#c3': 'nickname',
      '#c4': 'tags',
      '#c5': 'version',
    });
    assert.deepEqual(cmd.input.ExpressionAttributeValues, {
      ':u1': { S: 'v1' },
      ':c1': { S: 'v0' },
      ':c2': { N: '0' },
      ':c3': { N: '7' },
    } satisfies Record<string, AttributeValue>);
    assert.equal(cmd.input.ReturnValues, 'NONE');
    return { $metadata: {} };
  });

  const model = defineModel({
    name: 'C',
    table: { name: 't' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'name', type: 'S', optional: true },
      { attribute: 'count', type: 'N', optional: true },
      { attribute: 'nickname', type: 'S', optional: true },
      { attribute: 'tags', type: 'SS', optional: true },
      { attribute: 'version', type: 'N', roles: ['version'] },
    ],
  });

  const client = new TheorydbClient(mock.client).register(model);
  await client
    .updateBuilder('C', { PK: 'A' })
    .set('name', 'v1')
    .condition('name', '=', 'v0')
    .orCondition('count', '>', 0)
    .conditionExists('nickname')
    .conditionNotExists('tags')
    .conditionVersion(7)
    .execute();
}

{
  const mock = createMockDynamoDBClient();
  const provider = createDeterministicEncryptionProvider('seed');

  mock.when(UpdateItemCommand, async (cmd) => {
    assert.equal(cmd.input.UpdateExpression, 'SET #u1 = :u1');
    assert.deepEqual(cmd.input.ExpressionAttributeNames, { '#u1': 'secret' });
    assert.ok(cmd.input.ExpressionAttributeValues);
    const av = cmd.input.ExpressionAttributeValues[':u1'];
    assert.ok(av && 'M' in av);
    return { $metadata: {} };
  });

  const model = defineModel({
    name: 'E',
    table: { name: 't' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'secret', type: 'S', encryption: { v: 1 } },
    ],
  });

  const client = new TheorydbClient(mock.client, {
    encryption: provider,
  }).register(model);
  await client.updateBuilder('E', { PK: 'A' }).set('secret', 'x').execute();
}

{
  const mock = createMockDynamoDBClient();
  mock.when(UpdateItemCommand, async () => ({ $metadata: {} }));

  const model = defineModel({
    name: 'T',
    table: { name: 't' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'name', type: 'S', optional: true },
    ],
  });

  const client = new TheorydbClient(mock.client).register(model);
  await assert.rejects(
    () => client.updateBuilder('T', { PK: 'A' }).set('PK', 'nope').execute(),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
}

{
  const mock = createMockDynamoDBClient();
  mock.when(UpdateItemCommand, async () => ({ $metadata: {} }));
  const provider = createDeterministicEncryptionProvider('seed');

  const model = defineModel({
    name: 'E',
    table: { name: 't' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'secret', type: 'S', encryption: { v: 1 } },
    ],
  });

  const client = new TheorydbClient(mock.client, {
    encryption: provider,
  }).register(model);
  await assert.rejects(
    () =>
      client
        .updateBuilder('E', { PK: 'A' })
        .set('secret', 'x')
        .condition('secret', '=', 'y')
        .execute(),
    (e) =>
      e instanceof TheorydbError && e.code === 'ErrEncryptedFieldNotQueryable',
  );
}

{
  const mock = createMockDynamoDBClient();

  mock.when(UpdateItemCommand, async (cmd) => {
    assert.equal(
      cmd.input.UpdateExpression,
      'SET #u1 = if_not_exists(#u1, :u1)',
    );
    assert.deepEqual(cmd.input.ExpressionAttributeNames, { '#u1': 'name' });
    assert.deepEqual(cmd.input.ExpressionAttributeValues, {
      ':u1': { S: 'default' },
    } satisfies Record<string, AttributeValue>);
    assert.equal(cmd.input.ReturnValues, 'ALL_NEW');
    return {
      $metadata: {},
      Attributes: { PK: { S: 'A' }, name: { S: 'default' } },
    };
  });

  const model = defineModel({
    name: 'T',
    table: { name: 't' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'name', type: 'S', optional: true },
    ],
  });

  const client = new TheorydbClient(mock.client).register(model);
  const out = await client
    .updateBuilder('T', { PK: 'A' })
    .setIfNotExists('name', 'ignored', 'default')
    .returnValues('ALL_NEW')
    .execute();

  assert.deepEqual(out, { PK: 'A', name: 'default' });
}

{
  const mock = createMockDynamoDBClient();

  mock.when(UpdateItemCommand, async (cmd) => {
    assert.equal(cmd.input.UpdateExpression, 'REMOVE #u1');
    assert.deepEqual(cmd.input.ExpressionAttributeNames, { '#u1': 'nickname' });
    assert.equal(cmd.input.ExpressionAttributeValues, undefined);
    assert.equal(cmd.input.ConditionExpression, undefined);
    return { $metadata: {} };
  });

  const model = defineModel({
    name: 'T',
    table: { name: 't' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'nickname', type: 'S', optional: true },
    ],
  });

  const client = new TheorydbClient(mock.client).register(model);
  const out = await client
    .updateBuilder('T', { PK: 'A' })
    .remove('nickname')
    .execute();
  assert.equal(out, undefined);
}

{
  const mock = createMockDynamoDBClient();
  mock.when(UpdateItemCommand, async () => ({ $metadata: {} }));

  const model = defineModel({
    name: 'T',
    table: { name: 't' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'items', type: 'L', optional: true },
    ],
  });

  const client = new TheorydbClient(mock.client).register(model);

  await assert.rejects(
    () =>
      client
        .updateBuilder('T', { PK: 'A' })
        .removeFromListAt('items', -1)
        .execute(),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
  await assert.rejects(
    () =>
      client
        .updateBuilder('T', { PK: 'A' })
        .setListElement('items', -1, 'x')
        .execute(),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
}

{
  const mock = createMockDynamoDBClient();

  mock.when(UpdateItemCommand, async (cmd) => {
    assert.equal(cmd.input.UpdateExpression, 'ADD #u1 :u1, #u2 :u2, #u3 :u3');
    assert.deepEqual(cmd.input.ExpressionAttributeNames, {
      '#u1': 'tags',
      '#u2': 'nums',
      '#u3': 'blobs',
    });
    assert.ok(cmd.input.ExpressionAttributeValues);
    assert.deepEqual(cmd.input.ExpressionAttributeValues[':u1'], { SS: ['a'] });
    assert.deepEqual(cmd.input.ExpressionAttributeValues[':u2'], { NS: ['1'] });
    const b = cmd.input.ExpressionAttributeValues[':u3'];
    assert.ok(b && 'BS' in b && b.BS && b.BS[0] instanceof Uint8Array);
    return { $metadata: {} };
  });

  const model = defineModel({
    name: 'T',
    table: { name: 't' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'tags', type: 'SS', optional: true },
      { attribute: 'nums', type: 'NS', optional: true },
      { attribute: 'blobs', type: 'BS', optional: true },
    ],
  });

  const client = new TheorydbClient(mock.client).register(model);
  await client
    .updateBuilder('T', { PK: 'A' })
    .add('tags', 'a')
    .add('nums', 1)
    .add('blobs', new Uint8Array([1]))
    .execute();
}

{
  const mock = createMockDynamoDBClient();
  mock.when(UpdateItemCommand, async () => ({ $metadata: {} }));

  const model = defineModel({
    name: 'T',
    table: { name: 't' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'tags', type: 'SS', optional: true },
      { attribute: 'nums', type: 'NS', optional: true },
      { attribute: 'blobs', type: 'BS', optional: true },
      { attribute: 'name', type: 'S', optional: true },
      { attribute: 'count', type: 'N', optional: true },
    ],
  });

  const client = new TheorydbClient(mock.client).register(model);

  await assert.rejects(
    () =>
      client
        .updateBuilder('T', { PK: 'A' })
        .add('tags', 1 as never)
        .execute(),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
  await assert.rejects(
    () =>
      client
        .updateBuilder('T', { PK: 'A' })
        .add('nums', true as never)
        .execute(),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
  await assert.rejects(
    () =>
      client
        .updateBuilder('T', { PK: 'A' })
        .add('blobs', 'x' as never)
        .execute(),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
  await assert.rejects(
    () => client.updateBuilder('T', { PK: 'A' }).add('name', 'x').execute(),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
  await assert.rejects(
    () => client.updateBuilder('T', { PK: 'A' }).delete('count', 1).execute(),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
}

{
  const model = defineModel({
    name: 'E',
    table: { name: 't' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'secret', type: 'S', encryption: { v: 1 } },
    ],
  });

  const mock = createMockDynamoDBClient();
  const client = new TheorydbClient(mock.client).register(model);

  await assert.rejects(
    () => client.updateBuilder('E', { PK: 'A' }).set('secret', 'x').execute(),
    (e) =>
      e instanceof TheorydbError && e.code === 'ErrEncryptionNotConfigured',
  );
}

{
  const model = defineModel({
    name: 'T',
    table: { name: 't' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'name', type: 'S', optional: true },
    ],
  });

  const mock = createMockDynamoDBClient();
  const client = new TheorydbClient(mock.client).register(model);

  assert.throws(
    () =>
      client
        .updateBuilder('T', { PK: 'A' })
        .set('name', 'x')
        .conditionVersion(1),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidModel',
  );
}

{
  const mock = createMockDynamoDBClient();

  mock.when(UpdateItemCommand, async (cmd) => {
    assert.equal(cmd.input.UpdateExpression, 'SET #u1 = :u1');
    assert.equal(
      cmd.input.ConditionExpression,
      '#c1 BETWEEN :c1 AND :c2 AND #c2 IN (:c3, :c4) AND begins_with(#c2, :c5) OR contains(#c2, :c6)',
    );
    assert.deepEqual(cmd.input.ExpressionAttributeNames, {
      '#u1': 'name',
      '#c1': 'count',
      '#c2': 'name',
    });
    assert.ok(cmd.input.ExpressionAttributeValues);
    assert.deepEqual(cmd.input.ExpressionAttributeValues[':c1'], { N: '1' });
    assert.deepEqual(cmd.input.ExpressionAttributeValues[':c2'], { N: '2' });
    assert.deepEqual(cmd.input.ExpressionAttributeValues[':c3'], { S: 'a' });
    assert.deepEqual(cmd.input.ExpressionAttributeValues[':c4'], { S: 'b' });
    assert.deepEqual(cmd.input.ExpressionAttributeValues[':c5'], { S: 'a' });
    assert.deepEqual(cmd.input.ExpressionAttributeValues[':c6'], { S: 'x' });
    return { $metadata: {} };
  });

  const model = defineModel({
    name: 'T',
    table: { name: 't' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'name', type: 'S', optional: true },
      { attribute: 'count', type: 'N', optional: true },
    ],
  });

  const client = new TheorydbClient(mock.client).register(model);
  await client
    .updateBuilder('T', { PK: 'A' })
    .set('name', 'x')
    .condition('count', 'between', [1, 2])
    .condition('name', 'in', ['a', 'b'])
    .condition('name', 'begins_with', 'a')
    .orCondition('name', 'contains', 'x')
    .execute();
}

{
  const model = defineModel({
    name: 'T',
    table: { name: 't' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'name', type: 'S', optional: true },
      { attribute: 'count', type: 'N', optional: true },
    ],
  });

  const mock = createMockDynamoDBClient();
  mock.when(UpdateItemCommand, async () => ({ $metadata: {} }));

  const client = new TheorydbClient(mock.client).register(model);

  await assert.rejects(
    () =>
      client
        .updateBuilder('T', { PK: 'A' })
        .set('name', 'x')
        .condition('nope', '=', 'x')
        .execute(),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
  await assert.rejects(
    () =>
      client
        .updateBuilder('T', { PK: 'A' })
        .set('name', 'x')
        .condition('name', '=', undefined)
        .execute(),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
  await assert.rejects(
    () =>
      client
        .updateBuilder('T', { PK: 'A' })
        .set('name', 'x')
        .condition('count', 'IN', 1)
        .execute(),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
  await assert.rejects(
    () =>
      client
        .updateBuilder('T', { PK: 'A' })
        .set('name', 'x')
        .condition(
          'count',
          'IN',
          Array.from({ length: 101 }, (_, i) => i),
        )
        .execute(),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
  await assert.rejects(
    () =>
      client
        .updateBuilder('T', { PK: 'A' })
        .set('name', 'x')
        .condition('count', 'between', 1)
        .execute(),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
  await assert.rejects(
    () =>
      client
        .updateBuilder('T', { PK: 'A' })
        .set('name', 'x')
        .condition('count', 'bogus', 1)
        .execute(),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidOperator',
  );
}

{
  const mock = createMockDynamoDBClient();
  mock.when(UpdateItemCommand, async () => {
    throw { name: 'ConditionalCheckFailedException' };
  });

  const model = defineModel({
    name: 'T',
    table: { name: 't' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'name', type: 'S', optional: true },
    ],
  });

  const client = new TheorydbClient(mock.client).register(model);
  await assert.rejects(
    () => client.updateBuilder('T', { PK: 'A' }).set('name', 'x').execute(),
    (e) => e instanceof TheorydbError && e.code === 'ErrConditionFailed',
  );
}

{
  const mock = createMockDynamoDBClient();
  mock.when(UpdateItemCommand, async () => {
    throw { name: 'TransactionCanceledException' };
  });

  const model = defineModel({
    name: 'T',
    table: { name: 't' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'name', type: 'S', optional: true },
    ],
  });

  const client = new TheorydbClient(mock.client).register(model);
  await assert.rejects(
    () => client.updateBuilder('T', { PK: 'A' }).set('name', 'x').execute(),
    (e) => e instanceof TheorydbError && e.code === 'ErrConditionFailed',
  );
}

{
  const mock = createMockDynamoDBClient();
  const provider = createDeterministicEncryptionProvider('seed');

  const model = defineModel({
    name: 'E',
    table: { name: 't' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'secret', type: 'S', encryption: { v: 1 } },
    ],
  });

  const secretSchema = model.attributes.get('secret');
  assert.ok(secretSchema);
  const envelope = await encryptAttributeValue(
    secretSchema,
    'hello',
    provider,
    {
      model: model.name,
      attribute: 'secret',
    },
  );

  mock.when(UpdateItemCommand, async () => ({
    $metadata: {},
    Attributes: { PK: { S: 'A' }, secret: envelope },
  }));

  const client = new TheorydbClient(mock.client, {
    encryption: provider,
  }).register(model);
  const out = await client
    .updateBuilder('E', { PK: 'A' })
    .set('secret', 'hello')
    .returnValues('ALL_NEW')
    .execute();
  assert.deepEqual(out, { PK: 'A', secret: 'hello' });
}
