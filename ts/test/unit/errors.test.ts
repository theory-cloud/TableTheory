import test from 'node:test';
import assert from 'node:assert/strict';

import {
  isTheorydbError,
  safeErrorFields,
  TheorydbError,
  type ErrorCode,
} from '../../src/errors.js';

const releaseStateCodes: ErrorCode[] = [
  'ErrImmutableModelMutation',
  'ErrProtectedFieldMutation',
  'ErrRejectedDeployAuthorityEvidence',
];

void test('release-state errors use stable parity codes', () => {
  for (const code of releaseStateCodes) {
    const err = new TheorydbError(code, code);
    assert.equal(err.name, code);
    assert.equal(err.code, code);
    assert.ok(isTheorydbError(err));
  }
});

void test('safeErrorFields projects only the typed surface', () => {
  const err = new TheorydbError(
    'ErrTransactionFailed',
    'Transaction canceled',
    {
      cause: {
        CancellationReasons: [{ Item: { secret: { S: 'top-secret-value' } } }],
      },
      cancellationReasons: [{ index: 0, code: 'None', message: 'nope' }],
    },
  );

  const fields = safeErrorFields(err);
  assert.ok(fields);
  assert.equal(fields.code, 'ErrTransactionFailed');
  assert.deepEqual(fields.codes, ['ErrTransactionFailed']);
  assert.equal(fields.message, 'Transaction canceled');
  assert.deepEqual(fields.cancellationReasons, [
    { index: 0, code: 'None', message: 'nope' },
  ]);

  // Nothing reachable through the projection carries the item image, and the
  // projection has no `cause` at all.
  assert.ok(!JSON.stringify(fields).includes('top-secret-value'));
  assert.ok(!('cause' in fields));
});

void test('safeErrorFields returns undefined for a non-TheorydbError', () => {
  assert.equal(safeErrorFields(new Error('plain')), undefined);
  assert.equal(safeErrorFields('not an error'), undefined);
  assert.equal(safeErrorFields(undefined), undefined);
});
