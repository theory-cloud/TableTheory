import assert from 'node:assert/strict';

import { TheorydbError } from '../../src/errors.js';
import type { ModelSchema } from '../../src/model.js';
import { defineModel } from '../../src/model.js';

const userSchema: ModelSchema = {
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
    { attribute: 'tags', type: 'SS', optional: true, omit_empty: true },
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
    {
      attribute: 'ttl',
      type: 'N',
      format: 'unix_seconds',
      roles: ['ttl'],
      optional: true,
      omit_empty: true,
    },
  ],
  indexes: [
    {
      name: 'gsi-email',
      type: 'GSI',
      partition: { attribute: 'emailHash', type: 'S' },
      projection: { type: 'ALL' },
    },
  ],
};

{
  const schema: ModelSchema = {
    ...userSchema,
    attributes: [
      ...userSchema.attributes,
      { attribute: 'emailHash', type: 'S', optional: true },
    ],
  };

  const model = defineModel(schema);
  assert.equal(model.name, 'User');
  assert.equal(model.tableName, 'users_contract');
  assert.equal(model.roles.pk, 'PK');
  assert.equal(model.roles.sk, 'SK');
  assert.equal(model.roles.createdAt, 'createdAt');
  assert.equal(model.roles.updatedAt, 'updatedAt');
  assert.equal(model.roles.version, 'version');
  assert.equal(model.roles.ttl, 'ttl');
}

{
  const schema: ModelSchema = {
    ...userSchema,
    attributes: [...userSchema.attributes, { attribute: 'PK', type: 'S' }],
  };

  assert.throws(
    () => defineModel(schema),
    (err) => {
      assert.ok(err instanceof TheorydbError);
      assert.equal(err.code, 'ErrInvalidModel');
      return true;
    },
  );
}
