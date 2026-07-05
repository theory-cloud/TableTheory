import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { TheorydbError } from '../../src/errors.js';
import {
  assertModelsEquivalent,
  getDmsModel,
  modelToDmsModel,
  parseDmsDocument,
} from '../../src/dms.js';
import { defineModel } from '../../src/model.js';
import {
  DMSNoteModel,
  DMSNoteSchema,
} from '../fixtures/dms-codegen/generated-dms-note.js';

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
    write_policy:
      mode: "write_once"
      protected_attributes: ["value"]
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
  assert.equal(model.writePolicy.mode, 'write_once');
  assert.deepEqual(model.writePolicy.protectedAttributes, ['value']);
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
  const raw = readFileSync(
    new URL('../../../pkg/dms/testdata/codegen/dms-note.yml', import.meta.url),
    'utf8',
  );
  const doc = parseDmsDocument(raw);
  const dmsModel = getDmsModel(doc, 'DMSNote');

  assertModelsEquivalent(DMSNoteModel, dmsModel);
  assertModelsEquivalent(DMSNoteSchema, dmsModel);
  assert.equal(modelToDmsModel(DMSNoteModel).name, 'DMSNote');
  assert.equal(DMSNoteModel.roles.createdAt, 'createdAt');
  assert.deepEqual(DMSNoteModel.writePolicy.protectedAttributes, ['title']);
}

{
  const raw = readFileSync(
    new URL('../../../pkg/dms/testdata/codegen/dms-note.yml', import.meta.url),
    'utf8',
  );
  const doc = parseDmsDocument(raw);
  const dmsModel = getDmsModel(doc, 'DMSNote');
  const drifted = defineModel({
    ...DMSNoteSchema,
    write_policy: { mode: 'mutable', protected_attributes: ['count'] },
  });

  assert.throws(
    () => assertModelsEquivalent(drifted, dmsModel),
    (err) => {
      assert.ok(err instanceof TheorydbError);
      assert.equal(err.code, 'ErrInvalidModel');
      assert.match(err.message, /models not equivalent/);
      return true;
    },
  );
}

{
  const raw = `
dms_version: "0.1"
models:
  - name: "BadNaming"
    table: { name: "tbl" }
    naming: { convention: "pascalCase" }
    keys:
      partition: { attribute: "PK", type: "S" }
    attributes:
      - attribute: "PK"
        type: "S"
        required: true
        roles: ["pk"]
`;

  assert.throws(
    () => parseDmsDocument(raw),
    (err) => {
      assert.ok(err instanceof TheorydbError);
      assert.equal(err.code, 'ErrInvalidModel');
      assert.match(err.message, /unsupported naming\.convention/);
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
  - name: "BadPolicy"
    table: { name: "tbl" }
    keys:
      partition: { attribute: "PK", type: "S" }
    write_policy:
      mode: "mutable"
      protected_attributes: ["missing"]
    attributes:
      - attribute: "PK"
        type: "S"
        required: true
        roles: ["pk"]
`;

  assert.throws(
    () => parseDmsDocument(raw),
    (err) => {
      assert.ok(err instanceof TheorydbError);
      assert.equal(err.code, 'ErrInvalidModel');
      assert.match(err.message, /protected attribute not found/);
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
