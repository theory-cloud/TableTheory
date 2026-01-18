import assert from 'node:assert/strict';

import { TheorydbError } from '../../src/errors.js';
import { defineModel } from '../../src/model.js';
import {
  unmarshalStreamImage,
  unmarshalStreamRecord,
} from '../../src/streams.js';

const user = defineModel({
  name: 'User',
  table: { name: 'users_contract' },
  naming: { convention: 'camelCase' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', required: true, roles: ['pk'] },
    { attribute: 'SK', type: 'S', required: true, roles: ['sk'] },
    { attribute: 'nickname', type: 'S', optional: true, omit_empty: true },
    {
      attribute: 'createdAt',
      type: 'S',
      format: 'rfc3339nano',
      roles: ['created_at'],
    },
    {
      attribute: 'updatedAt',
      type: 'S',
      format: 'rfc3339nano',
      roles: ['updated_at'],
    },
    { attribute: 'version', type: 'N', format: 'int', roles: ['version'] },
  ],
});

const newImage = {
  PK: { S: 'USER#1' },
  SK: { S: 'PROFILE' },
  nickname: { S: 'Al' },
  createdAt: { S: '2026-01-16T00:00:00.000000000Z' },
  updatedAt: { S: '2026-01-16T00:00:00.000000000Z' },
  version: { N: '0' },
};

{
  const item = unmarshalStreamImage(user, newImage);
  assert.equal(item.PK, 'USER#1');
  assert.equal(item.SK, 'PROFILE');
  assert.equal(item.nickname, 'Al');
  assert.equal(item.version, 0);
}

{
  const partial = unmarshalStreamImage(user, {
    PK: { S: 'USER#1' },
    SK: { S: 'PROFILE' },
    version: { N: '1' },
  });
  assert.ok(!('nickname' in partial));
}

{
  assert.throws(
    () =>
      unmarshalStreamImage(user, {
        PK: { S: 'USER#1' },
        SK: { S: 'PROFILE' },
        version: { N: 1 } as unknown,
      }),
    (err) => err instanceof TheorydbError,
  );
}

{
  const rec = unmarshalStreamRecord(user, {
    dynamodb: {
      Keys: { PK: { S: 'USER#1' }, SK: { S: 'PROFILE' } },
      NewImage: newImage,
    },
  });
  assert.equal(rec.keys?.PK, 'USER#1');
  assert.equal(rec.newImage?.version, 0);
  assert.equal(rec.oldImage, undefined);
}
