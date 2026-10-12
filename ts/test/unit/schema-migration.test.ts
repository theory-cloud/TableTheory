import assert from 'node:assert/strict';
import test from 'node:test';

import {
  BatchWriteItemCommand,
  DescribeTableCommand,
  ScanCommand,
  type AttributeValue,
  type DynamoDBClient,
} from '@aws-sdk/client-dynamodb';

import { decodeEncryptedPayload } from '../../src/encryption-avjson.js';
import type { EncryptionProvider } from '../../src/encryption.js';
import { TheorydbError } from '../../src/errors.js';
import { defineModel } from '../../src/model.js';
import {
  addField,
  autoMigrate,
  chainTransforms,
  copyAllFields,
  removeField,
  renameField,
} from '../../src/schema-migration.js';

class StubDdb {
  readonly commands: unknown[] = [];
  constructor(private readonly handler: (cmd: unknown) => unknown) {}
  async send(cmd: unknown): Promise<unknown> {
    this.commands.push(cmd);
    return this.handler(cmd);
  }
}

const envelope = (): AttributeValue => ({
  M: {
    v: { N: '1' },
    edk: { B: new Uint8Array([1, 2, 3]) },
    nonce: { B: new Uint8Array([4, 5, 6]) },
    ct: { B: new Uint8Array([7, 8, 9]) },
  },
});

function isEnvelope(av: AttributeValue | undefined): boolean {
  return (
    av !== undefined &&
    'M' in av &&
    av.M !== undefined &&
    av.M.v?.N === '1' &&
    av.M.edk?.B !== undefined &&
    av.M.edk.B.length > 0 &&
    av.M.nonce?.B !== undefined &&
    av.M.nonce.B.length > 0 &&
    av.M.ct?.B !== undefined &&
    av.M.ct.B.length > 0
  );
}

const plainSource = defineModel({
  name: 'PlainSource',
  table: { name: 'migration_source' },
  keys: { partition: { attribute: 'PK', type: 'S' } },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'secret', type: 'S' },
  ],
});

const encryptedTarget = defineModel({
  name: 'EncryptedTarget',
  table: { name: 'migration_target' },
  keys: { partition: { attribute: 'PK', type: 'S' } },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'secret', type: 'S', encryption: { v: 1 } },
  ],
});

const encryptedSource = defineModel({
  name: 'EncryptedSource',
  table: { name: 'migration_enc_source' },
  keys: { partition: { attribute: 'PK', type: 'S' } },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'old', type: 'S', encryption: { v: 1 } },
  ],
});

const encryptedTargetRenamed = defineModel({
  name: 'EncryptedTargetRenamed',
  table: { name: 'migration_enc_target' },
  keys: { partition: { attribute: 'PK', type: 'S' } },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'new', type: 'S', encryption: { v: 1 } },
  ],
});

const encryptedSourceX = defineModel({
  name: 'EncryptedSourceX',
  table: { name: 'migration_enc_x_source' },
  keys: { partition: { attribute: 'PK', type: 'S' } },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'x', type: 'S', encryption: { v: 1 } },
  ],
});

const plainTargetX = defineModel({
  name: 'PlainTargetX',
  table: { name: 'migration_plain_x_target' },
  keys: { partition: { attribute: 'PK', type: 'S' } },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'x', type: 'S' },
  ],
});

const plainTargetA = defineModel({
  name: 'PlainTargetA',
  table: { name: 'migration_plain_a' },
  keys: { partition: { attribute: 'PK', type: 'S' } },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'name', type: 'S' },
  ],
});

const plainSourceA = defineModel({
  name: 'PlainSourceA',
  table: { name: 'migration_plain_source' },
  keys: { partition: { attribute: 'PK', type: 'S' } },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'name', type: 'S' },
  ],
});

function writeItemFor(
  ddb: StubDdb,
  tableName: string,
): Record<string, AttributeValue> | undefined {
  const writes = ddb.commands.filter(
    (cmd): cmd is BatchWriteItemCommand => cmd instanceof BatchWriteItemCommand,
  );
  return writes[0]?.input.RequestItems?.[tableName]?.[0]?.PutRequest?.Item;
}

const item: Record<string, AttributeValue> = {
  PK: { S: 'USER#1' },
  SK: { S: 'v1' },
  name: { S: 'Ada' },
};

void test('copyAllFields passes attributes through unchanged', () => {
  const out = copyAllFields()(item);
  assert.deepEqual(out, item);
  assert.notEqual(out, item);
});

void test('renameField renames one attribute and preserves its value', () => {
  const out = renameField('name', 'displayName')(item);
  assert.deepEqual(out, {
    PK: { S: 'USER#1' },
    SK: { S: 'v1' },
    displayName: { S: 'Ada' },
  });
});

void test('addField adds an attribute', () => {
  const out = addField('status', { S: 'active' })(item);
  assert.deepEqual(out.status, { S: 'active' });
  assert.deepEqual(out.name, { S: 'Ada' });
});

void test('removeField drops an attribute', () => {
  const out = removeField('name')(item);
  assert.equal('name' in out, false);
  assert.deepEqual(out.PK, { S: 'USER#1' });
});

void test('chainTransforms composes transforms left to right', () => {
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

const unusedProvider: EncryptionProvider = {
  async encrypt() {
    throw new Error('provider must not be called');
  },
  async decrypt() {
    throw new Error('not used');
  },
};

function describeActive(cmd: unknown): unknown {
  if (cmd instanceof DescribeTableCommand) {
    return { Table: { TableStatus: 'ACTIVE' } };
  }
  return undefined;
}

void test('S1: migration requiring encryption without a provider fails closed', async () => {
  const ddb = new StubDdb(() => {
    throw new Error('unexpected command');
  });

  await assert.rejects(
    () =>
      autoMigrate(ddb as unknown as DynamoDBClient, plainSource, {
        targetModel: encryptedTarget,
        dataCopy: true,
      }),
    (err) => {
      assert.ok(err instanceof TheorydbError);
      assert.equal(err.code, 'ErrMigrationEncryptionRequired');
      return true;
    },
  );

  assert.equal(ddb.commands.length, 0);
});

void test('S2: migration encrypts plaintext into a target encrypted attribute', async () => {
  const provider: EncryptionProvider = {
    async encrypt(plaintext, ctx) {
      assert.equal(ctx.model, 'EncryptedTarget');
      assert.equal(ctx.attribute, 'secret');
      assert.deepEqual(decodeEncryptedPayload(plaintext), { S: 'top-secret' });
      return {
        v: 1,
        edk: new Uint8Array([1, 2, 3]),
        nonce: new Uint8Array([4, 5, 6]),
        ct: new Uint8Array([9, 9, 9]),
      };
    },
    async decrypt() {
      throw new Error('not used');
    },
  };

  const ddb = new StubDdb((cmd) => {
    const table = describeActive(cmd);
    if (table !== undefined) return table;
    if (cmd instanceof ScanCommand) {
      return { Items: [{ PK: { S: 'USER#1' }, secret: { S: 'top-secret' } }] };
    }
    if (cmd instanceof BatchWriteItemCommand) return {};
    throw new Error('unexpected command');
  });

  await autoMigrate(ddb as unknown as DynamoDBClient, plainSource, {
    targetModel: encryptedTarget,
    dataCopy: true,
    encryption: provider,
  });

  const written = writeItemFor(ddb, 'migration_target');
  assert.ok(written);
  assert.ok(isEnvelope(written.secret));
  assert.equal(JSON.stringify(written).includes('top-secret'), false);
});

void test('S3: already-enveloped values are not re-encrypted', async () => {
  const ddb = new StubDdb((cmd) => {
    const table = describeActive(cmd);
    if (table !== undefined) return table;
    if (cmd instanceof ScanCommand) {
      return { Items: [{ PK: { S: 'USER#1' }, old: envelope() }] };
    }
    throw new Error('unexpected command');
  });

  await assert.rejects(
    () =>
      autoMigrate(ddb as unknown as DynamoDBClient, encryptedSource, {
        targetModel: encryptedTargetRenamed,
        transform: renameField('old', 'new'),
        dataCopy: true,
        encryption: unusedProvider,
      }),
    (err) => {
      assert.ok(err instanceof TheorydbError);
      assert.equal(err.code, 'ErrMigrationEncryptionRequired');
      assert.match(err.message, /already an encrypted envelope/);
      return true;
    },
  );

  assert.equal(
    ddb.commands.filter((cmd) => cmd instanceof BatchWriteItemCommand).length,
    0,
  );
});

void test('S3: encrypted source attribute cannot be copied into a plaintext target', async () => {
  const ddb = new StubDdb((cmd) => {
    const table = describeActive(cmd);
    if (table !== undefined) return table;
    if (cmd instanceof ScanCommand) {
      return { Items: [{ PK: { S: 'USER#1' }, x: envelope() }] };
    }
    throw new Error('unexpected command');
  });

  await assert.rejects(
    () =>
      autoMigrate(ddb as unknown as DynamoDBClient, encryptedSourceX, {
        targetModel: plainTargetX,
        dataCopy: true,
        encryption: unusedProvider,
      }),
    (err) => {
      assert.ok(err instanceof TheorydbError);
      assert.equal(err.code, 'ErrMigrationEncryptionRequired');
      assert.match(
        err.message,
        /into a target attribute that is not encrypted/,
      );
      return true;
    },
  );

  assert.equal(
    ddb.commands.filter((cmd) => cmd instanceof BatchWriteItemCommand).length,
    0,
  );
});

void test('S4: migrations without encrypted transitions copy data unchanged', async () => {
  const ddb = new StubDdb((cmd) => {
    const table = describeActive(cmd);
    if (table !== undefined) return table;
    if (cmd instanceof ScanCommand) {
      return { Items: [{ PK: { S: 'USER#1' }, name: { S: 'Ada' } }] };
    }
    if (cmd instanceof BatchWriteItemCommand) return {};
    throw new Error('unexpected command');
  });

  await autoMigrate(ddb as unknown as DynamoDBClient, plainSourceA, {
    targetModel: plainTargetA,
    dataCopy: true,
  });

  const written = writeItemFor(ddb, 'migration_plain_a');
  assert.deepEqual(written, { PK: { S: 'USER#1' }, name: { S: 'Ada' } });
});

void test('S4: an encrypted target without a data copy is not refused', async () => {
  const ddb = new StubDdb((cmd) => {
    const table = describeActive(cmd);
    if (table !== undefined) return table;
    throw new Error('unexpected command');
  });

  await autoMigrate(ddb as unknown as DynamoDBClient, encryptedTarget);

  assert.ok(
    ddb.commands.some((cmd) => cmd instanceof DescribeTableCommand),
    'ensureTable must still run for a table-only migration',
  );
  assert.equal(
    ddb.commands.filter((cmd) => cmd instanceof BatchWriteItemCommand).length,
    0,
  );
});
