import assert from 'node:assert/strict';

import {
  ConditionalCheckFailedException,
  DeleteItemCommand,
  TransactWriteItemsCommand,
  TransactionCanceledException,
  type DynamoDBClient,
} from '@aws-sdk/client-dynamodb';

import { TheorydbError } from '../../src/errors.js';
import { FaceTheoryIsrMetaStore } from '../../src/facetheory-isr.js';
import { createMockDynamoDBClient } from '../../src/testkit/index.js';

const leaseToken = 'lease-tok';
const nowMs = 1_000_000;

function commitArgs() {
  return {
    cacheKey: 'a',
    leaseToken,
    nowMs,
    htmlPointer: 's3://bucket/key.html',
    generatedAtMs: nowMs,
    revalidateSeconds: 60,
    etag: '"abc"',
  };
}

{
  const mock = createMockDynamoDBClient();
  mock.when(TransactWriteItemsCommand, async () => ({ $metadata: {} }));

  const store = new FaceTheoryIsrMetaStore({
    ddb: mock.client as unknown as DynamoDBClient,
    tableName: 'tbl',
  });

  await store.commitGeneration(commitArgs());

  assert.equal(mock.calls.length, 1);
  const cmd = mock.calls[0];
  assert.ok(cmd instanceof TransactWriteItemsCommand);

  const items = cmd.input.TransactItems ?? [];
  assert.equal(items.length, 2);

  const puts = items.filter((it) => it.Put !== undefined);
  const deletes = items.filter((it) => it.Delete !== undefined);
  const conditionChecks = items.filter((it) => it.ConditionCheck !== undefined);

  assert.equal(puts.length, 1);
  assert.equal(deletes.length, 1);
  assert.equal(conditionChecks.length, 0);

  const meta = puts[0]?.Put;
  assert.equal(meta?.TableName, 'tbl');
  assert.equal(meta?.Item?.pk?.S, 'CACHE#a');
  assert.equal(meta?.Item?.sk?.S, 'META');
  assert.equal(meta?.Item?.s3_key?.S, 's3://bucket/key.html');

  const lock = deletes[0]?.Delete;
  assert.equal(lock?.TableName, 'tbl');
  assert.equal(lock?.Key?.pk?.S, 'CACHE#a');
  assert.equal(lock?.Key?.sk?.S, 'LOCK');

  const condition = lock?.ConditionExpression ?? '';
  assert.match(condition, /#tok\s*=\s*:tok/);
  assert.match(condition, /#exp\s*>\s*:now/);
  assert.equal(lock?.ExpressionAttributeNames?.['#tok'], 'lease_token');
  assert.equal(lock?.ExpressionAttributeNames?.['#exp'], 'lease_expires_at');
  assert.equal(lock?.ExpressionAttributeValues?.[':tok']?.S, leaseToken);
  assert.equal(lock?.ExpressionAttributeValues?.[':now']?.N, '1000');
}

for (const [label, makeError] of [
  [
    'TransactionCanceledException',
    () => new TransactionCanceledException({ $metadata: {}, message: 'no' }),
  ],
  [
    'ConditionalCheckFailedException',
    () => new ConditionalCheckFailedException({ $metadata: {}, message: 'no' }),
  ],
] as const) {
  const mock = createMockDynamoDBClient();
  mock.when(TransactWriteItemsCommand, async () => {
    throw makeError();
  });

  const store = new FaceTheoryIsrMetaStore({
    ddb: mock.client as unknown as DynamoDBClient,
    tableName: 'tbl',
  });

  await assert.rejects(
    () => store.commitGeneration(commitArgs()),
    (e) => e instanceof TheorydbError && e.code === 'ErrLeaseNotOwned',
    label,
  );

  assert.equal(mock.calls.length, 1);
  const cmd = mock.calls[0];
  assert.ok(cmd instanceof TransactWriteItemsCommand);

  const deletes = (cmd.input.TransactItems ?? []).filter(
    (it) => it.Delete !== undefined,
  );
  assert.equal(deletes.length, 1);
  assert.ok(deletes[0]?.Delete?.ConditionExpression);
  assert.equal(deletes[0]?.Delete?.Key?.sk?.S, 'LOCK');

  assert.equal(
    mock.calls.some((c) => c instanceof DeleteItemCommand),
    false,
  );
}
