import assert from 'node:assert/strict';

import {
  decodeEncryptedPayload,
  encodeEncryptedPayload,
} from '../../src/encryption-avjson.js';
import { TheorydbError } from '../../src/errors.js';

{
  const bytes = encodeEncryptedPayload({ S: 'x' });
  assert.equal(Buffer.from(bytes).toString('utf8'), '{"t":"S","s":"x"}');
  assert.deepEqual(decodeEncryptedPayload(bytes), { S: 'x' });
}

{
  const bytes = encodeEncryptedPayload({ N: '1' });
  assert.equal(Buffer.from(bytes).toString('utf8'), '{"t":"N","n":"1"}');
  assert.deepEqual(decodeEncryptedPayload(bytes), { N: '1' });
}

{
  const bytes = encodeEncryptedPayload({ BOOL: true });
  assert.equal(Buffer.from(bytes).toString('utf8'), '{"t":"BOOL","bool":true}');
  assert.deepEqual(decodeEncryptedPayload(bytes), { BOOL: true });
}

{
  const bytes = encodeEncryptedPayload({ NULL: true });
  assert.equal(Buffer.from(bytes).toString('utf8'), '{"t":"NULL","null":true}');
  assert.deepEqual(decodeEncryptedPayload(bytes), { NULL: true });
}

{
  const bytes = encodeEncryptedPayload({ SS: ['a', 'b'] });
  assert.deepEqual(decodeEncryptedPayload(bytes), { SS: ['a', 'b'] });
}

{
  const bytes = encodeEncryptedPayload({ NS: ['1', '2'] });
  assert.deepEqual(decodeEncryptedPayload(bytes), { NS: ['1', '2'] });
}

{
  const bytes = encodeEncryptedPayload({ B: new Uint8Array([1, 2, 3]) });
  const decoded = decodeEncryptedPayload(bytes);
  assert.ok('B' in decoded);
  assert.deepEqual(Buffer.from(decoded.B), Buffer.from([1, 2, 3]));
}

{
  const bytes = encodeEncryptedPayload({
    BS: [new Uint8Array([1]), new Uint8Array([2])],
  });
  const decoded = decodeEncryptedPayload(bytes);
  assert.ok('BS' in decoded);
  assert.deepEqual(
    decoded.BS.map((b) => Buffer.from(b)),
    [Buffer.from([1]), Buffer.from([2])],
  );
}

{
  const bytes = encodeEncryptedPayload({ L: [{ S: 'x' }, { N: '1' }] });
  assert.deepEqual(decodeEncryptedPayload(bytes), {
    L: [{ S: 'x' }, { N: '1' }],
  });
}

{
  const bytes = encodeEncryptedPayload({
    M: { b: { S: 'x' }, a: { N: '1' } },
  });
  assert.equal(
    Buffer.from(bytes).toString('utf8'),
    '{"t":"M","m":{"a":{"t":"N","n":"1"},"b":{"t":"S","s":"x"}}}',
  );
  assert.deepEqual(decodeEncryptedPayload(bytes), {
    M: { a: { N: '1' }, b: { S: 'x' } },
  });
}

{
  assert.throws(
    () => decodeEncryptedPayload(Buffer.from('{bad-json', 'utf8')),
    (err) =>
      err instanceof TheorydbError &&
      err.code === 'ErrInvalidEncryptedEnvelope',
  );

  assert.throws(
    () => decodeEncryptedPayload(Buffer.from('{}', 'utf8')),
    (err) =>
      err instanceof TheorydbError &&
      err.code === 'ErrInvalidEncryptedEnvelope',
  );

  assert.throws(
    () => decodeEncryptedPayload(Buffer.from('{"t":"X"}', 'utf8')),
    (err) =>
      err instanceof TheorydbError &&
      err.code === 'ErrInvalidEncryptedEnvelope',
  );

  assert.throws(
    () => encodeEncryptedPayload({} as unknown as never),
    (err) =>
      err instanceof TheorydbError &&
      err.code === 'ErrInvalidEncryptedEnvelope',
  );

  assert.throws(
    () =>
      encodeEncryptedPayload({
        M: { a: undefined as unknown as never },
      } as unknown as never),
    (err) =>
      err instanceof TheorydbError &&
      err.code === 'ErrInvalidEncryptedEnvelope',
  );
}
