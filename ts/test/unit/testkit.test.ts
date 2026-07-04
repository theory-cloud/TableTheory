import test from 'node:test';
import assert from 'node:assert/strict';

import {
  BatchGetItemCommand,
  BatchWriteItemCommand,
  CreateTableCommand,
  DeleteItemCommand,
  DeleteTableCommand,
  DescribeTableCommand,
  GetItemCommand,
  ListTablesCommand,
  PutItemCommand,
  QueryCommand,
  ResourceInUseException,
  ScanCommand,
  TransactionCanceledException,
  TransactGetItemsCommand,
  TransactWriteItemsCommand,
  UpdateItemCommand,
  UpdateTimeToLiveCommand,
  type AttributeValue,
} from '@aws-sdk/client-dynamodb';

import { TheorydbClient } from '../../src/client.js';
import { defineModel } from '../../src/model.js';
import {
  createDeterministicEncryptionProvider,
  createMockDynamoDBClient,
  createStatefulDynamoDBClient,
  fixedNow,
} from '../../src/testkit/index.js';
import { hasTheorydbErrorCode } from '../../src/errors.js';

function assertInstanceOf<T>(
  value: unknown,
  ctor: new (input: never) => T,
): asserts value is T {
  assert.ok(value instanceof ctor);
}

function assertDefined<T>(value: T): asserts value is NonNullable<T> {
  assert.ok(value !== undefined && value !== null);
}

function avS(value: string): AttributeValue {
  return { S: value };
}

function avN(value: string): AttributeValue {
  return { N: value };
}

function statefulKey(pk: string, sk: string): Record<string, AttributeValue> {
  return { PK: avS(pk), SK: avS(sk) };
}

function statefulItem(
  pk: string,
  sk: string,
  name: string,
  score: string,
): Record<string, AttributeValue> {
  return {
    ...statefulKey(pk, sk),
    name: avS(name),
    score: avN(score),
    tag: avS(name.slice(0, 1)),
    ttl: avN('1700000000'),
  };
}

void test('createMockDynamoDBClient is strict by default', async () => {
  const mock = createMockDynamoDBClient();
  await assert.rejects(() =>
    mock.client.send(new PutItemCommand({ TableName: 't', Item: {} })),
  );
});

void test('createMockDynamoDBClient matches handlers by command name', async () => {
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

void test('client now() injection drives createdAt/updatedAt + update :now', async () => {
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

void test('createDeterministicEncryptionProvider round-trips + binds AAD to attribute', async () => {
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

void test('createStatefulDynamoDBClient stores, queries, and checks versions', async () => {
  const stateful = createStatefulDynamoDBClient();
  const model = defineModel({
    name: 'T',
    table: { name: 'stateful_t' },
    keys: {
      partition: { attribute: 'PK', type: 'S' },
      sort: { attribute: 'SK', type: 'S' },
    },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'SK', type: 'S', roles: ['sk'] },
      { attribute: 'emailHash', type: 'S', optional: true },
      { attribute: 'name', type: 'S', optional: true },
      { attribute: 'createdAt', type: 'S', roles: ['created_at'] },
      { attribute: 'updatedAt', type: 'S', roles: ['updated_at'] },
      { attribute: 'version', type: 'N', roles: ['version'] },
      { attribute: 'ttl', type: 'N', roles: ['ttl'], optional: true },
    ],
  });
  const now = '2026-07-04T00:00:00.000000000Z';
  const client = new TheorydbClient(stateful.client, {
    now: fixedNow(now),
  }).register(model);

  await client.create('T', {
    PK: 'USER#1',
    SK: 'PROFILE',
    emailHash: 'test@example',
    name: 'one',
    ttl: 1_700_000_000,
  });

  const page = await client
    .query('T')
    .partitionKey('USER#1')
    .sortKey('begins_with', 'PRO')
    .filter('emailHash', '=', 'test@example')
    .all();
  assert.equal(page.length, 1);
  assert.equal(page[0]?.name, 'one');
  assert.equal(page[0]?.version, 0);
  assert.equal(page[0]?.createdAt, now);

  await client.update(
    'T',
    { PK: 'USER#1', SK: 'PROFILE', name: 'two', version: 0 },
    ['name'],
  );
  await assert.rejects(
    () =>
      client.update(
        'T',
        { PK: 'USER#1', SK: 'PROFILE', name: 'stale', version: 0 },
        ['name'],
      ),
    (err) =>
      hasTheorydbErrorCode(err, 'ErrConditionFailed') &&
      hasTheorydbErrorCode(err, 'ErrVersionConflict'),
  );

  const got = await client.get('T', { PK: 'USER#1', SK: 'PROFILE' });
  assert.equal(got.name, 'two');
  assert.equal(got.version, 1);
});

void test('StatefulDynamoDBFake supports admin, batches, scans, and transactions', async () => {
  const { client, fake } = createStatefulDynamoDBClient();
  const tableName = 'stateful_admin';

  await client.send(
    new CreateTableCommand({
      TableName: tableName,
      KeySchema: [
        { AttributeName: 'PK', KeyType: 'HASH' },
        { AttributeName: 'SK', KeyType: 'RANGE' },
      ],
    }),
  );
  await assert.rejects(
    () => client.send(new CreateTableCommand({ TableName: tableName })),
    ResourceInUseException,
  );
  await client.send(
    new UpdateTimeToLiveCommand({
      TableName: tableName,
      TimeToLiveSpecification: { AttributeName: 'ttl', Enabled: true },
    }),
  );

  const listed = (await client.send(new ListTablesCommand({ Limit: 1 }))) as {
    TableNames?: string[];
  };
  assert.deepEqual(listed.TableNames, [tableName]);

  fake.seed(
    tableName,
    statefulItem('USER#1', 'A', 'one', '10'),
    statefulItem('USER#1', 'B', 'two', '20'),
    statefulItem('USER#2', 'A', 'three', '30'),
  );

  const described = (await client.send(
    new DescribeTableCommand({ TableName: tableName }),
  )) as { Table?: { ItemCount?: number } };
  assert.equal(described.Table?.ItemCount, 3);

  const got = (await client.send(
    new GetItemCommand({
      TableName: tableName,
      Key: statefulKey('USER#1', 'A'),
      ProjectionExpression: 'PK, #n',
      ExpressionAttributeNames: { '#n': 'name' },
    }),
  )) as { Item?: Record<string, AttributeValue> };
  assert.deepEqual(got.Item?.name, avS('one'));
  assert.equal(got.Item?.score, undefined);

  const scanned = (await client.send(
    new ScanCommand({
      TableName: tableName,
      FilterExpression: '(#score >= :min AND begins_with(#name, :prefix))',
      ExpressionAttributeNames: { '#name': 'name', '#score': 'score' },
      ExpressionAttributeValues: { ':min': avN('10'), ':prefix': avS('t') },
      Limit: 1,
    }),
  )) as {
    Items?: Array<Record<string, AttributeValue>>;
    LastEvaluatedKey?: Record<string, AttributeValue>;
  };
  assert.equal(scanned.Items?.length, 1);
  assert.ok(scanned.LastEvaluatedKey);

  const counted = (await client.send(
    new ScanCommand({ TableName: tableName, Select: 'COUNT' }),
  )) as { Count?: number; Items?: unknown[] };
  assert.equal(counted.Count, 3);
  assert.equal(counted.Items, undefined);

  const queryPage = (await client.send(
    new QueryCommand({
      TableName: tableName,
      KeyConditionExpression: '#pk = :pk AND #sk >= :sk',
      ExpressionAttributeNames: { '#pk': 'PK', '#sk': 'SK' },
      ExpressionAttributeValues: { ':pk': avS('USER#1'), ':sk': avS('A') },
      ScanIndexForward: false,
      Limit: 1,
    }),
  )) as {
    Items?: Array<Record<string, AttributeValue>>;
    LastEvaluatedKey?: Record<string, AttributeValue>;
  };
  assert.deepEqual(queryPage.Items?.[0]?.SK, avS('B'));

  const nextQueryPage = (await client.send(
    new QueryCommand({
      TableName: tableName,
      KeyConditionExpression: '#pk = :pk AND #sk >= :sk',
      ExpressionAttributeNames: { '#pk': 'PK', '#sk': 'SK' },
      ExpressionAttributeValues: { ':pk': avS('USER#1'), ':sk': avS('A') },
      ExclusiveStartKey: queryPage.LastEvaluatedKey,
      ScanIndexForward: false,
    }),
  )) as { Items?: Array<Record<string, AttributeValue>> };
  assert.deepEqual(nextQueryPage.Items?.[0]?.SK, avS('A'));

  await client.send(
    new BatchWriteItemCommand({
      RequestItems: {
        [tableName]: [
          { PutRequest: { Item: statefulItem('USER#1', 'C', 'four', '40') } },
          { DeleteRequest: { Key: statefulKey('USER#1', 'B') } },
        ],
      },
    }),
  );
  const batch = (await client.send(
    new BatchGetItemCommand({
      RequestItems: {
        [tableName]: {
          Keys: [statefulKey('USER#1', 'A'), statefulKey('USER#1', 'C')],
        },
      },
    }),
  )) as { Responses?: Record<string, unknown[]> };
  assert.equal(batch.Responses?.[tableName]?.length, 2);

  const updated = (await client.send(
    new UpdateItemCommand({
      TableName: tableName,
      Key: statefulKey('USER#1', 'C'),
      UpdateExpression: 'SET #note = :note REMOVE #tag ADD #score :inc',
      ExpressionAttributeNames: {
        '#note': 'note',
        '#score': 'score',
        '#tag': 'tag',
      },
      ExpressionAttributeValues: { ':inc': avN('2'), ':note': avS('seeded') },
      ReturnValues: 'ALL_NEW',
    }),
  )) as { Attributes?: Record<string, AttributeValue> };
  assert.deepEqual(updated.Attributes?.score, avN('42'));
  assert.deepEqual(updated.Attributes?.note, avS('seeded'));
  assert.equal(updated.Attributes?.tag, undefined);

  const transactRead = (await client.send(
    new TransactGetItemsCommand({
      TransactItems: [
        { Get: { TableName: tableName, Key: statefulKey('USER#1', 'C') } },
        { Get: { TableName: tableName, Key: statefulKey('MISSING', 'A') } },
      ],
    }),
  )) as { Responses?: Array<{ Item?: Record<string, AttributeValue> }> };
  assert.ok(transactRead.Responses?.[0]?.Item);
  assert.equal(transactRead.Responses?.[1]?.Item, undefined);

  await client.send(
    new TransactWriteItemsCommand({
      TransactItems: [
        {
          Put: {
            TableName: tableName,
            Item: statefulItem('TX#1', 'A', 'tx', '1'),
          },
        },
        {
          ConditionCheck: {
            TableName: tableName,
            Key: statefulKey('USER#1', 'C'),
            ConditionExpression: 'attribute_exists(PK)',
          },
        },
        {
          Update: {
            TableName: tableName,
            Key: statefulKey('TX#1', 'A'),
            UpdateExpression: 'ADD #score :inc',
            ExpressionAttributeNames: { '#score': 'score' },
            ExpressionAttributeValues: { ':inc': avN('1') },
          },
        },
        { Delete: { TableName: tableName, Key: statefulKey('USER#2', 'A') } },
      ],
    }),
  );
  await assert.rejects(
    () =>
      client.send(
        new TransactWriteItemsCommand({
          TransactItems: [
            {
              ConditionCheck: {
                TableName: tableName,
                Key: statefulKey('USER#1', 'C'),
                ConditionExpression: 'attribute_not_exists(PK)',
              },
            },
          ],
        }),
      ),
    TransactionCanceledException,
  );

  await client.send(
    new DeleteItemCommand({
      TableName: tableName,
      Key: statefulKey('USER#1', 'A'),
      ConditionExpression: 'attribute_exists(PK)',
    }),
  );
  await client.send(new DeleteTableCommand({ TableName: tableName }));
  await assert.rejects(
    () => client.send(new DescribeTableCommand({ TableName: tableName })),
    /not found/,
  );
  fake.reset();
  assert.deepEqual(fake.items(tableName), []);
});
