import assert from 'node:assert/strict';

import { TheorydbError } from '../../src/errors.js';
import { defineModel } from '../../src/model.js';
import {
  isEmpty,
  marshalKey,
  marshalDocumentValue,
  marshalPutItem,
  marshalScalar,
  nowRfc3339Nano,
  unmarshalItem,
  unmarshalDocumentValue,
  unmarshalScalar,
} from '../../src/marshal.js';

assert.equal(
  nowRfc3339Nano(new Date('2026-01-16T00:00:00.123Z')),
  '2026-01-16T00:00:00.123000000Z',
);

assert.equal(isEmpty(null), true);
assert.equal(isEmpty(undefined), true);
assert.equal(isEmpty(''), true);
assert.equal(isEmpty(0), true);
assert.equal(isEmpty(false), true);
assert.equal(isEmpty([]), true);
assert.equal(isEmpty({}), true);
assert.equal(isEmpty({ a: '' }), true);
assert.equal(isEmpty({ a: 'x' }), false);

assert.deepEqual(marshalScalar({ attribute: 'S', type: 'S' }, 'x'), { S: 'x' });
assert.deepEqual(marshalScalar({ attribute: 'N', type: 'N' }, 1), { N: '1' });
assert.deepEqual(marshalScalar({ attribute: 'N', type: 'N' }, 1n), { N: '1' });
assert.deepEqual(marshalScalar({ attribute: 'N', type: 'N' }, '1'), { N: '1' });
assert.deepEqual(
  marshalScalar(
    { attribute: 'payload', type: 'S', json: true },
    { b: 2, a: 1 },
  ),
  { S: '{"a":1,"b":2}' },
);
assert.deepEqual(
  marshalScalar({ attribute: 'payload', type: 'S', json: true }, null),
  {
    NULL: true,
  },
);
assert.deepEqual(
  marshalScalar({ attribute: 'B', type: 'B' }, Buffer.from('a')),
  {
    B: Buffer.from('a'),
  },
);
assert.deepEqual(marshalScalar({ attribute: 'SS', type: 'SS' }, ['a', 'b']), {
  SS: ['a', 'b'],
});
assert.deepEqual(marshalScalar({ attribute: 'SS', type: 'SS' }, []), {
  NULL: true,
});
assert.deepEqual(marshalScalar({ attribute: 'NS', type: 'NS' }, [1, 2]), {
  NS: ['1', '2'],
});
assert.deepEqual(marshalScalar({ attribute: 'NS', type: 'NS' }, []), {
  NULL: true,
});
assert.deepEqual(
  marshalScalar({ attribute: 'BS', type: 'BS' }, [Buffer.from('a')]),
  {
    BS: [Buffer.from('a')],
  },
);
assert.deepEqual(marshalScalar({ attribute: 'BS', type: 'BS' }, []), {
  NULL: true,
});
assert.deepEqual(
  marshalScalar({ attribute: 'L', type: 'L' }, ['a', 1, true, null]),
  { L: [{ S: 'a' }, { N: '1' }, { BOOL: true }, { NULL: true }] },
);
assert.deepEqual(
  marshalScalar(
    { attribute: 'M', type: 'M' },
    { a: 1, b: 'x', nested: { c: true } },
  ),
  { M: { a: { N: '1' }, b: { S: 'x' }, nested: { M: { c: { BOOL: true } } } } },
);
assert.deepEqual(marshalScalar({ attribute: 'BOOL', type: 'BOOL' }, true), {
  BOOL: true,
});
assert.deepEqual(marshalScalar({ attribute: 'NULL', type: 'NULL' }, 123), {
  NULL: true,
});

assert.throws(() => marshalScalar({ attribute: 'S', type: 'S' }, 1));
assert.throws(() => marshalScalar({ attribute: 'N', type: 'N' }, false));
assert.throws(() => marshalScalar({ attribute: 'B', type: 'B' }, 'x'));
assert.throws(() =>
  marshalScalar({ attribute: 'SS', type: 'SS' }, ['a', 1] as never),
);
assert.throws(() =>
  marshalScalar({ attribute: 'NS', type: 'NS' }, [true] as never),
);
assert.throws(() =>
  marshalScalar({ attribute: 'BS', type: 'BS' }, ['x'] as never),
);
assert.throws(() =>
  marshalScalar({ attribute: 'L', type: 'L' }, { a: 1 } as never),
);
assert.throws(() =>
  marshalScalar({ attribute: 'M', type: 'M' }, ['x'] as never),
);
assert.throws(() => marshalScalar({ attribute: 'X', type: 'X' as never }, 'x'));

assert.equal(unmarshalScalar({ attribute: 'S', type: 'S' }, { S: 'x' }), 'x');
assert.equal(unmarshalScalar({ attribute: 'N', type: 'N' }, { N: '1' }), 1);
assert.deepEqual(
  unmarshalScalar(
    { attribute: 'payload', type: 'S', json: true },
    { S: '{"a":1,"b":2}' },
  ),
  { a: 1, b: 2 },
);
assert.equal(
  unmarshalScalar(
    { attribute: 'payload', type: 'S', json: true },
    { NULL: true },
  ),
  null,
);
assert.deepEqual(
  unmarshalScalar({ attribute: 'B', type: 'B' }, { B: Buffer.from('a') }),
  Buffer.from('a'),
);
assert.deepEqual(
  unmarshalScalar({ attribute: 'SS', type: 'SS' }, { SS: ['a'] }),
  ['a'],
);
assert.deepEqual(
  unmarshalScalar({ attribute: 'SS', type: 'SS' }, { NULL: true }),
  [],
);
assert.deepEqual(
  unmarshalScalar({ attribute: 'NS', type: 'NS' }, { NS: ['1'] }),
  [1],
);
assert.deepEqual(
  unmarshalScalar({ attribute: 'NS', type: 'NS' }, { NULL: true }),
  [],
);
assert.deepEqual(
  unmarshalScalar({ attribute: 'BS', type: 'BS' }, { BS: [Buffer.from('a')] }),
  [Buffer.from('a')],
);
assert.deepEqual(
  unmarshalScalar({ attribute: 'BS', type: 'BS' }, { NULL: true }),
  [],
);
assert.deepEqual(
  unmarshalScalar(
    { attribute: 'L', type: 'L' },
    { L: [{ S: 'x' }, { N: '1' }] },
  ),
  ['x', 1],
);
assert.deepEqual(
  unmarshalScalar({ attribute: 'M', type: 'M' }, { M: { a: { N: '1' } } }),
  { a: 1 },
);
assert.equal(
  unmarshalScalar({ attribute: 'BOOL', type: 'BOOL' }, { BOOL: false }),
  false,
);
assert.equal(
  unmarshalScalar({ attribute: 'NULL', type: 'NULL' }, { NULL: true }),
  null,
);
assert.throws(() =>
  unmarshalScalar({ attribute: 'S', type: 'S' }, { NS: ['1'] } as never),
);
assert.throws(() =>
  marshalScalar({ attribute: 'payload', type: 'S', json: true }, {
    x: undefined,
  } as never),
);
assert.throws(() =>
  unmarshalScalar({ attribute: 'payload', type: 'S', json: true }, {
    S: '{',
  } as never),
);

{
  class ID {
    constructor(readonly raw: string) {}
  }
  const schema = {
    attribute: 'id',
    type: 'S',
    converter: {
      toDynamoValue: (value: unknown): unknown => {
        assert.ok(value instanceof ID);
        return value.raw;
      },
      fromDynamoValue: (value: unknown): unknown => new ID(String(value)),
    },
  } as const;

  assert.deepEqual(marshalScalar(schema, new ID('abc')), { S: 'abc' });
  const out = unmarshalScalar(schema, { S: 'abc' });
  assert.ok(out instanceof ID);
  assert.equal(out.raw, 'abc');
}

assert.throws(() => marshalDocumentValue(undefined as never));
assert.deepEqual(unmarshalDocumentValue({ NULL: true }), null);

const User = defineModel({
  name: 'User',
  table: { name: 'users_contract' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'nickname', type: 'S', optional: true, omit_empty: true },
    { attribute: 'createdAt', type: 'S', roles: ['created_at'] },
    { attribute: 'updatedAt', type: 'S', roles: ['updated_at'] },
    { attribute: 'version', type: 'N', roles: ['version'] },
  ],
});

assert.throws(
  () => marshalKey(User, { SK: 'B' }),
  (err) => err instanceof TheorydbError && err.code === 'ErrMissingPrimaryKey',
);

const key = marshalKey(User, { PK: 'A', SK: 'B' });
assert.deepEqual(key, { PK: { S: 'A' }, SK: { S: 'B' } });

assert.throws(
  () => marshalPutItem(User, { PK: 'A', SK: 'B', nope: 'x' } as never),
  (err) => err instanceof TheorydbError && err.code === 'ErrInvalidModel',
);

{
  const put = marshalPutItem(
    User,
    { PK: 'A', SK: 'B', nickname: '' },
    { now: '2026-01-16T00:00:00.000000000Z' },
  );
  assert.equal(put.createdAt?.S, '2026-01-16T00:00:00.000000000Z');
  assert.equal(put.updatedAt?.S, '2026-01-16T00:00:00.000000000Z');
  assert.equal(put.version?.N, '0');
  assert.ok(!('nickname' in put));
}

{
  const raw = {
    PK: { S: 'A' },
    SK: { S: 'B' },
    version: { N: '1' },
    extra: { S: 'raw' },
  };
  const item = unmarshalItem(User, raw);
  assert.equal(item.PK, 'A');
  assert.equal(item.SK, 'B');
  assert.equal(item.version, 1);
  assert.deepEqual(item.extra, { S: 'raw' });
}
