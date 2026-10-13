import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  ConditionalCheckFailedException,
  TransactionCanceledException,
} from '@aws-sdk/client-dynamodb';

import { mapDynamoError } from '../../src/dynamo-error.js';
import { TheorydbError } from '../../src/errors.js';

function canceled(reasons?: unknown[]): TransactionCanceledException {
  return new TransactionCanceledException({
    $metadata: {},
    message: 'transaction canceled',
    ...(reasons !== undefined ? { CancellationReasons: reasons as never } : {}),
  });
}

void test('transaction cancellation classifies from the first genuine failure', () => {
  const err = mapDynamoError(
    canceled([
      { Code: 'None' },
      { Code: 'ConditionalCheckFailed', Message: 'loser' },
    ]),
  ) as TheorydbError;

  assert.ok(err instanceof TheorydbError);
  assert.equal(err.code, 'ErrConditionFailed');
  assert.deepEqual(err.cancellationReasons, [
    { index: 0, code: 'None', message: '' },
    { index: 1, code: 'ConditionalCheckFailed', message: 'loser' },
  ]);
});

void test('transaction cancellation keeps every reason in order and uses the first real failure', () => {
  const exception = canceled([
    { Code: 'None' },
    { Code: 'TransactionConflict', Message: 'raced' },
    { Code: 'ConditionalCheckFailed', Message: 'loser' },
  ]);
  const err = mapDynamoError(exception) as TheorydbError;

  // The first genuine failure wins; a later condition failure must not override.
  assert.equal(err.code, 'ErrTransactionConflict');
  assert.deepEqual(
    err.cancellationReasons.map((r) => r.code),
    ['None', 'TransactionConflict', 'ConditionalCheckFailed'],
  );
  assert.equal((err as { cause?: unknown }).cause, exception);
});

void test('transaction cancellation classifies throttling and conflicts', () => {
  const throttled = mapDynamoError(
    canceled([{ Code: 'ThrottlingError' }]),
  ) as TheorydbError;
  assert.equal(throttled.code, 'ErrThrottled');

  const throughput = mapDynamoError(
    canceled([{ Code: 'ProvisionedThroughputExceeded' }]),
  ) as TheorydbError;
  assert.equal(throughput.code, 'ErrThrottled');

  const conflict = mapDynamoError(
    canceled([{ Code: 'TransactionConflict' }]),
  ) as TheorydbError;
  assert.equal(conflict.code, 'ErrTransactionConflict');
});

void test('an unmapped or missing reason code is a generic failure, never a conflict', () => {
  const unknown = mapDynamoError(
    canceled([{ Code: 'SomeUnmappedReason' }]),
  ) as TheorydbError;
  assert.equal(unknown.code, 'ErrTransactionFailed');
  assert.notEqual(unknown.code, 'ErrConditionFailed');

  const noReasons = mapDynamoError(canceled()) as TheorydbError;
  assert.equal(noReasons.code, 'ErrTransactionFailed');
  assert.deepEqual(noReasons.cancellationReasons, []);

  const allNone = mapDynamoError(
    canceled([{ Code: 'None' }, { Code: 'None' }]),
  ) as TheorydbError;
  assert.equal(allNone.code, 'ErrTransactionFailed');

  const missingCode = mapDynamoError(
    canceled([{ Message: 'no code here' }, { Code: 'ConditionalCheckFailed' }]),
  ) as TheorydbError;
  assert.equal(missingCode.code, 'ErrConditionFailed');
  assert.deepEqual(
    missingCode.cancellationReasons.map((r) => r.code),
    ['', 'ConditionalCheckFailed'],
  );
});

void test('transaction cancellation never exposes an item image', () => {
  const err = mapDynamoError(
    canceled([
      {
        Code: 'ConditionalCheckFailed',
        Message: 'conditional request failed',
        Item: { secret: { S: 'top-secret-value' } },
      },
    ]),
  ) as TheorydbError;

  assert.ok(
    !JSON.stringify(err.cancellationReasons).includes('top-secret-value'),
  );
  assert.ok(!err.message.includes('top-secret-value'));
});

void test('duck-typed transaction cancellation still classifies by reason', () => {
  const err = mapDynamoError({
    name: 'TransactionCanceledException',
    CancellationReasons: [{ Code: 'ConditionalCheckFailed' }],
  }) as TheorydbError;

  assert.ok(err instanceof TheorydbError);
  assert.equal(err.code, 'ErrConditionFailed');
  assert.equal(err.cancellationReasons[0]?.code, 'ConditionalCheckFailed');
});

void test('an ambiguous transport error is not reclassified', () => {
  const transport = new Error(
    'dial tcp: connection refused (ConditionalCheckFailed inside the message)',
  );

  assert.equal(mapDynamoError(transport), transport);
  assert.equal(
    mapDynamoError(
      new ConditionalCheckFailedException({ $metadata: {}, message: 'x' }),
    ) instanceof TheorydbError,
    true,
  );
});
