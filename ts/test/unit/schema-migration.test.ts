import assert from 'node:assert/strict';
import test from 'node:test';

import type { AttributeValue } from '@aws-sdk/client-dynamodb';

import {
  addField,
  chainTransforms,
  copyAllFields,
  removeField,
  renameField,
} from '../../src/schema-migration.js';

const item: Record<string, AttributeValue> = {
  PK: { S: 'USER#1' },
  SK: { S: 'v1' },
  name: { S: 'Ada' },
};

test('copyAllFields passes attributes through unchanged', () => {
  const out = copyAllFields()(item);
  assert.deepEqual(out, item);
  assert.notEqual(out, item);
});

test('renameField renames one attribute and preserves its value', () => {
  const out = renameField('name', 'displayName')(item);
  assert.deepEqual(out, {
    PK: { S: 'USER#1' },
    SK: { S: 'v1' },
    displayName: { S: 'Ada' },
  });
});

test('addField adds an attribute', () => {
  const out = addField('status', { S: 'active' })(item);
  assert.deepEqual(out.status, { S: 'active' });
  assert.deepEqual(out.name, { S: 'Ada' });
});

test('removeField drops an attribute', () => {
  const out = removeField('name')(item);
  assert.equal('name' in out, false);
  assert.deepEqual(out.PK, { S: 'USER#1' });
});

test('chainTransforms composes transforms left to right', () => {
  const out = chainTransforms(
    renameField('name', 'displayName'),
    addField('status', { S: 'active' }),
    removeField('SK'),
  )(item);
  assert.deepEqual(out, {
    PK: { S: 'USER#1' },
    displayName: { S: 'Ada' },
    status: { S: 'active' },
  });
});
