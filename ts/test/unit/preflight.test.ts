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

const BASE_ITEM = { PK: 'P#1', SK: 'S#1' };

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
