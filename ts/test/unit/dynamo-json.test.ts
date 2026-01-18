import assert from 'node:assert/strict';

import { fromDynamoJson, toDynamoJson } from '../../src/dynamo-json.js';

assert.deepEqual(toDynamoJson({ S: 'x' }), { S: 'x' });
assert.deepEqual(toDynamoJson({ N: '1' }), { N: '1' });
assert.deepEqual(toDynamoJson({ BOOL: true }), { BOOL: true });
assert.deepEqual(toDynamoJson({ NULL: true }), { NULL: true });
assert.deepEqual(toDynamoJson({ SS: ['a', 'b'] }), { SS: ['a', 'b'] });
assert.deepEqual(toDynamoJson({ NS: ['1', '2'] }), { NS: ['1', '2'] });

{
  const json = toDynamoJson({ B: Buffer.from('hi') });
  assert.deepEqual(json, { B: Buffer.from('hi').toString('base64') });
  assert.deepEqual(fromDynamoJson(json), { B: Buffer.from('hi') });
}

{
  const json = toDynamoJson({ BS: [Buffer.from('a'), Buffer.from('b')] });
  assert.deepEqual(json, {
    BS: [
      Buffer.from('a').toString('base64'),
      Buffer.from('b').toString('base64'),
    ],
  });
  assert.deepEqual(fromDynamoJson(json), {
    BS: [Buffer.from('a'), Buffer.from('b')],
  });
}

{
  const json = toDynamoJson({
    L: [{ S: 'x' }, { N: '1' }, { BOOL: false }, { NULL: true }],
  });
  assert.deepEqual(fromDynamoJson(json), {
    L: [{ S: 'x' }, { N: '1' }, { BOOL: false }, { NULL: true }],
  });
}

{
  const json = toDynamoJson({ M: { a: { S: 'x' }, b: { N: '1' } } });
  assert.deepEqual(fromDynamoJson(json), {
    M: { a: { S: 'x' }, b: { N: '1' } },
  });
}

assert.throws(() => toDynamoJson({ M: { a: undefined as unknown } } as never));

assert.throws(() => fromDynamoJson(null));
assert.throws(() => fromDynamoJson([]));
assert.throws(() => fromDynamoJson({}));
assert.throws(() => fromDynamoJson({ S: 'x', N: '1' }));
assert.throws(() => fromDynamoJson({ S: 1 }));
assert.throws(() => fromDynamoJson({ N: 1 }));
assert.throws(() => fromDynamoJson({ BOOL: 'true' }));
assert.throws(() => fromDynamoJson({ NULL: false }));
assert.throws(() => fromDynamoJson({ SS: ['a', 1] }));
assert.throws(() => fromDynamoJson({ NS: [1] }));
assert.throws(() => fromDynamoJson({ B: 1 }));
assert.throws(() => fromDynamoJson({ BS: [1] }));
assert.throws(() => fromDynamoJson({ L: 'x' }));
assert.throws(() => fromDynamoJson({ M: [] }));
assert.throws(() => fromDynamoJson({ X: 'x' }));
