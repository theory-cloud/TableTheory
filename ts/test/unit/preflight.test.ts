import assert from 'node:assert/strict';
import test from 'node:test';

import type { AttributeValue } from '@aws-sdk/client-dynamodb';

import { TheorydbClient } from '../../src/client.js';
import { TheorydbError } from '../../src/errors.js';
import { defineModel } from '../../src/model.js';
import {
  marshalItemForWriterPath,
  type ItemWriterPath,
} from '../../src/preflight.js';
import {
  createStatefulDynamoDBClient,
  fixedNow,
} from '../../src/testkit/index.js';

const NOW = '2026-01-16T00:00:00.123000000Z';

// The matrix model deliberately declares no lifecycle fields and no custom
// converters, so both writer paths produce a deterministic payload that can be
// compared byte for byte against a captured one.
const PreflightMatrix = defineModel({
  name: 'PreflightMatrix',
  table: { name: 'preflight_matrix' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'rootList', type: 'L', optional: true },
    { attribute: 'rootMap', type: 'M', optional: true },
    { attribute: 'rootListOmit', type: 'L', optional: true, omit_empty: true },
    { attribute: 'rootMapOmit', type: 'M', optional: true, omit_empty: true },
    { attribute: 'flag', type: 'BOOL', optional: true },
    { attribute: 'score', type: 'N', optional: true },
    { attribute: 'nested', type: 'M', optional: true },
  ],
});

const StampedMatrix = defineModel({
  name: 'StampedMatrix',
  table: { name: 'preflight_stamped' },
  keys: { partition: { attribute: 'PK', type: 'S' } },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'name', type: 'S', optional: true },
    { attribute: 'createdAt', type: 'S', roles: ['created_at'] },
    { attribute: 'updatedAt', type: 'S', roles: ['updated_at'] },
    { attribute: 'version', type: 'N', roles: ['version'] },
  ],
});

const EncryptedMatrix = defineModel({
  name: 'EncryptedMatrix',
  table: { name: 'preflight_encrypted' },
  keys: { partition: { attribute: 'PK', type: 'S' } },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'secret', type: 'S', encryption: { v: 1 } },
  ],
});

function requireOne(
  items: readonly Record<string, AttributeValue>[],
  message: string,
): Record<string, AttributeValue> {
  const [first] = items;
  assert.ok(first !== undefined, message);
  assert.equal(items.length, 1, message);
  return first;
}

async function captureCreate(
  item: Record<string, unknown>,
): Promise<Record<string, AttributeValue>> {
  const { client, fake } = createStatefulDynamoDBClient();
  const theorydb = new TheorydbClient(client, { now: fixedNow(NOW) }).register(
    PreflightMatrix,
  );
  await theorydb.create('PreflightMatrix', item);
  return requireOne(
    fake.items('preflight_matrix'),
    'the fake must have stored exactly one put item',
  );
}

async function captureTransactWritePut(
  item: Record<string, unknown>,
): Promise<Record<string, AttributeValue>> {
  const { client, fake } = createStatefulDynamoDBClient();
  const theorydb = new TheorydbClient(client, { now: fixedNow(NOW) }).register(
    PreflightMatrix,
  );
  await theorydb.transactWrite([
    { kind: 'put', model: 'PreflightMatrix', item },
  ]);
  return requireOne(
    fake.items('preflight_matrix'),
    'the fake must have stored exactly one transacted item',
  );
}

// The pin the issue asks for: each answer must equal a payload captured from the
// real writer path it claims to predict. TypeScript funnels create, save,
// batchWrite puts and transactWrite puts through one marshalPutItem call, so the
// two captures must also agree with each other.
void test('marshalItemForWriterPath byte-equals captured create and transactWrite payloads', async () => {
  const item = {
    PK: 'P#1',
    SK: 'S#1',
    rootList: ['a', 1, true, null],
    rootMap: { a: 1, b: { c: 'x' } },
    rootListOmit: [],
    rootMapOmit: {},
    flag: false,
    score: 0,
    nested: {
      nullEntry: null,
      emptyArray: [],
      emptyObject: {},
      deep: { k: 'v' },
    },
  };

  const capturedPut = await captureCreate(item);
  const capturedTransact = await captureTransactWritePut(item);

  const preflightPut = marshalItemForWriterPath(PreflightMatrix, item, 'put');
  const preflightTransact = marshalItemForWriterPath(
    PreflightMatrix,
    item,
    'transact_write',
  );

  assert.deepEqual(
    preflightPut,
    capturedPut,
    'put preflight must equal the captured create payload',
  );
  assert.deepEqual(
    preflightTransact,
    capturedTransact,
    'transact_write preflight must equal the captured transactWrite payload',
  );
  assert.deepEqual(
    capturedTransact,
    capturedPut,
    'both real writers funnel through marshalPutItem, so their payloads agree',
  );
  assert.deepEqual(
    preflightTransact,
    preflightPut,
    'TypeScript has one item serializer, so both preflight answers agree',
  );

  assert.deepEqual(capturedPut.rootList, {
    L: [{ S: 'a' }, { N: '1' }, { BOOL: true }, { NULL: true }],
  });
  assert.ok(
    !('rootListOmit' in capturedPut),
    'omit_empty drops an empty root list on both writer paths',
  );
  assert.ok(
    !('rootMapOmit' in capturedPut),
    'omit_empty drops an empty root map on both writer paths',
  );
});

// The preflight's `opts.now` and the client's injected clock are the same stamp,
// so a lifecycle-stamped model also round-trips through a captured create.
void test('marshalItemForWriterPath matches a captured create payload for lifecycle fields', async () => {
  const item = { PK: 'P#1', name: 'first' };
  const { client, fake } = createStatefulDynamoDBClient();
  const theorydb = new TheorydbClient(client, { now: fixedNow(NOW) }).register(
    StampedMatrix,
  );
  await theorydb.create('StampedMatrix', item);

  const captured = requireOne(
    fake.items('preflight_stamped'),
    'the fake must have stored exactly one put item',
  );
  const preflight = marshalItemForWriterPath(StampedMatrix, item, 'put', {
    now: NOW,
  });

  assert.deepEqual(preflight, captured);
  assert.deepEqual(preflight.createdAt, { S: NOW });
  assert.deepEqual(preflight.updatedAt, { S: NOW });
  assert.deepEqual(preflight.version, { N: '0' });
});

function attrType(av: AttributeValue | undefined): string {
  if (av === undefined) return 'ABSENT';
  if ('NULL' in av && av.NULL) return 'NULL';
  if ('L' in av) return 'L';
  if ('M' in av) return 'M';
  if ('S' in av) return 'S';
  if ('N' in av) return 'N';
  if ('BOOL' in av) return 'BOOL';
  return 'OTHER';
}

function rootReader(name: string) {
  return (item: Record<string, AttributeValue>): AttributeValue | undefined =>
    item[name];
}

function nestedReader(name: string) {
  return (item: Record<string, AttributeValue>): AttributeValue | undefined => {
    const container = item['nested'];
    if (container === undefined || !('M' in container)) return undefined;
    return container.M?.[name];
  };
}

type MatrixRow = {
  name: string;
  item: Record<string, unknown>;
  read: (item: Record<string, AttributeValue>) => AttributeValue | undefined;
  put: string;
  transactWrite: string;
  exact?: AttributeValue;
};

const BASE_ITEM = { PK: 'P#1', SK: 'S#1' };

// TypeScript has no nil collection: a collection value is an array or an object,
// and the nearest input to Go's nil slice is an absent property. An absent
// property is dropped by marshalPutItem (`value === undefined` -> omitted), not
// stored as NULL, and both writer paths share that one rule.
void test('marshalItemForWriterPath pins the nil/empty semantics of both writer paths', () => {
  const rows: MatrixRow[] = [
    {
      name: 'absent root list without omit_empty',
      item: { ...BASE_ITEM },
      read: rootReader('rootList'),
      put: 'ABSENT',
      transactWrite: 'ABSENT',
    },
    {
      name: 'empty root list without omit_empty',
      item: { ...BASE_ITEM, rootList: [] },
      read: rootReader('rootList'),
      put: 'L',
      transactWrite: 'L',
      exact: { L: [] },
    },
    {
      name: 'absent root map without omit_empty',
      item: { ...BASE_ITEM },
      read: rootReader('rootMap'),
      put: 'ABSENT',
      transactWrite: 'ABSENT',
    },
    {
      name: 'empty root map without omit_empty',
      item: { ...BASE_ITEM, rootMap: {} },
      read: rootReader('rootMap'),
      put: 'M',
      transactWrite: 'M',
      exact: { M: {} },
    },
    {
      name: 'absent root list with omit_empty',
      item: { ...BASE_ITEM },
      read: rootReader('rootListOmit'),
      put: 'ABSENT',
      transactWrite: 'ABSENT',
    },
    {
      name: 'empty root list with omit_empty',
      item: { ...BASE_ITEM, rootListOmit: [] },
      read: rootReader('rootListOmit'),
      put: 'ABSENT',
      transactWrite: 'ABSENT',
    },
    {
      name: 'absent root map with omit_empty',
      item: { ...BASE_ITEM },
      read: rootReader('rootMapOmit'),
      put: 'ABSENT',
      transactWrite: 'ABSENT',
    },
    {
      name: 'empty root map with omit_empty',
      item: { ...BASE_ITEM, rootMapOmit: {} },
      read: rootReader('rootMapOmit'),
      put: 'ABSENT',
      transactWrite: 'ABSENT',
    },
    {
      name: 'explicit false boolean without omit_empty',
      item: { ...BASE_ITEM, flag: false },
      read: rootReader('flag'),
      put: 'BOOL',
      transactWrite: 'BOOL',
      exact: { BOOL: false },
    },
    {
      name: 'zero number without omit_empty',
      item: { ...BASE_ITEM, score: 0 },
      read: rootReader('score'),
      put: 'N',
      transactWrite: 'N',
      exact: { N: '0' },
    },
    {
      name: 'nested null entry',
      item: { ...BASE_ITEM, nested: { nullEntry: null } },
      read: nestedReader('nullEntry'),
      put: 'NULL',
      transactWrite: 'NULL',
      exact: { NULL: true },
    },
    {
      name: 'nested empty array',
      item: { ...BASE_ITEM, nested: { emptyArray: [] } },
      read: nestedReader('emptyArray'),
      put: 'L',
      transactWrite: 'L',
      exact: { L: [] },
    },
    {
      name: 'nested empty object',
      item: { ...BASE_ITEM, nested: { emptyObject: {} } },
      read: nestedReader('emptyObject'),
      put: 'M',
      transactWrite: 'M',
      exact: { M: {} },
    },
  ];

  for (const row of rows) {
    const putItem = marshalItemForWriterPath(PreflightMatrix, row.item, 'put');
    const transactItem = marshalItemForWriterPath(
      PreflightMatrix,
      row.item,
      'transact_write',
    );

    assert.equal(attrType(row.read(putItem)), row.put, `${row.name}: put path`);
    assert.equal(
      attrType(row.read(transactItem)),
      row.transactWrite,
      `${row.name}: transact_write path`,
    );

    if (row.exact !== undefined) {
      assert.deepEqual(row.read(putItem), row.exact, `${row.name}: put value`);
      assert.deepEqual(
        row.read(transactItem),
        row.exact,
        `${row.name}: transact_write value`,
      );
    }
  }
});

void test('marshalItemForWriterPath refuses an unknown writer path', () => {
  for (const path of ['nope', '']) {
    assert.throws(
      () =>
        marshalItemForWriterPath(
          PreflightMatrix,
          { ...BASE_ITEM },
          path as unknown as ItemWriterPath,
        ),
      (err) => err instanceof TheorydbError && err.code === 'ErrInvalidModel',
    );
  }
});

void test('marshalItemForWriterPath refuses a model with encrypted attributes', () => {
  for (const path of ['put', 'transact_write'] as const) {
    assert.throws(
      () =>
        marshalItemForWriterPath(
          EncryptedMatrix,
          { PK: 'P#1', secret: 'plaintext' },
          path,
        ),
      (err) => err instanceof TheorydbError && err.code === 'ErrInvalidModel',
    );
  }
});
