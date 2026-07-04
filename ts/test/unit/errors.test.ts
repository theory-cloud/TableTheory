import test from 'node:test';
import assert from 'node:assert/strict';

import {
  isTheorydbError,
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
