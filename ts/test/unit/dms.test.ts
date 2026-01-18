import assert from 'node:assert/strict';

import { TheorydbError } from '../../src/errors.js';
import { getDmsModel, parseDmsDocument } from '../../src/dms.js';
import { defineModel } from '../../src/model.js';

{
  const raw = `
dms_version: "0.1"
namespace: "theorydb.test"
models:
  - name: "Demo"
    table: { name: "tbl" }
    keys:
      partition: { attribute: "PK", type: "S" }
      sort: { attribute: "SK", type: "S" }
    attributes:
      - attribute: "PK"
        type: "S"
        required: true
        roles: ["pk"]
      - attribute: "SK"
        type: "S"
        required: true
        roles: ["sk"]
      - attribute: "value"
        type: "S"
        optional: true
        omit_empty: true
`;

  const doc = parseDmsDocument(raw);
  const schema = getDmsModel(doc, 'Demo');
  const model = defineModel(schema);
  assert.equal(model.name, 'Demo');
  assert.equal(model.tableName, 'tbl');
  assert.equal(model.roles.pk, 'PK');
  assert.equal(model.roles.sk, 'SK');
}

{
  const raw = `
dms_version: "0.2"
models: []
`;

  assert.throws(
    () => parseDmsDocument(raw),
    (err) => {
      assert.ok(err instanceof TheorydbError);
      assert.equal(err.code, 'ErrInvalidModel');
      return true;
    },
  );
}

{
  const raw = `
dms_version: "0.1"
models:
  - name: "Demo"
    table: { name: "tbl" }
    keys:
      partition: { attribute: "PK", type: "S" }
    attributes:
      - attribute: "PK"
        type: "S"
        required: true
        roles: ["pk"]
payload: !!binary "Zm9v"
`;

  assert.throws(
    () => parseDmsDocument(raw),
    (err) => {
      assert.ok(err instanceof TheorydbError);
      assert.equal(err.code, 'ErrInvalidModel');
      return true;
    },
  );
}

{
  const raw = `
dms_version: "0.1"
models:
  - name: "Demo"
    table: { name: "tbl" }
    keys:
      partition: { attribute: "PK", type: "S" }
    attributes:
      - attribute: "PK"
        type: "S"
        required: true
        roles: ["pk"]
`;

  const doc = parseDmsDocument(raw);
  assert.throws(
    () => getDmsModel(doc, 'Missing'),
    (err) => {
      assert.ok(err instanceof TheorydbError);
      assert.equal(err.code, 'ErrInvalidModel');
      return true;
    },
  );
}
