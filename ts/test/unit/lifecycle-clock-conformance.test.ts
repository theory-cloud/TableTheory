import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import test from 'node:test';

import type { AttributeValue } from '@aws-sdk/client-dynamodb';
import { parse } from 'yaml';

import { TheorydbClient } from '../../src/client.js';
import { defineModel } from '../../src/model.js';
import {
  marshalItemForWriterPath,
  type ItemWriterPath,
} from '../../src/preflight.js';
import {
  createStatefulDynamoDBClient,
  fixedNow,
} from '../../src/testkit/index.js';

const TABLE_NAME = 'lifecycle_clocks';
const BASE_ITEM = { PK: 'P#1', SK: 'S#1' };

// The shared lifecycle-clock table the Go, TypeScript and Python suites all
// consume. It is resolved from this file's own URL so the path holds however the
// suite is launched.
const LIFECYCLE_TABLE_PATH = new URL(
  '../../../contract-tests/serialization/lifecycle-clocks.yml',
  import.meta.url,
);

type LifecycleRow = {
  mode: string;
  expect: Record<string, Record<string, string>>;
  put_equals_transact_write?: Record<string, boolean>;
  exact_equality_supported: Record<string, boolean>;
};

type LifecycleTable = {
  version: number;
  name: string;
  writer_paths: string[];
  runtimes: string[];
  clock_modes: string[];
  stamps: Record<string, string>;
  expectations: string[];
  rows: LifecycleRow[];
};

const LifecycleClocks = defineModel({
  name: 'LifecycleClocks',
  table: { name: TABLE_NAME },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'createdAt', type: 'S', roles: ['created_at'] },
    { attribute: 'updatedAt', type: 'S', roles: ['updated_at'] },
  ],
});

function loadLifecycleTable(): LifecycleTable {
  assert.ok(
    existsSync(LIFECYCLE_TABLE_PATH),
    `the shared lifecycle table must exist at ${LIFECYCLE_TABLE_PATH.pathname}`,
  );
  const table = parse(
    readFileSync(LIFECYCLE_TABLE_PATH, 'utf8'),
  ) as LifecycleTable;
  assert.equal(table.version, 1);
  assert.ok(Array.isArray(table.rows) && table.rows.length > 0);
  return table;
}

// One clock instance drives both writer calls, so an advancing clock observes
// the sequence the fixture describes rather than a per-call restart.
function scenarioClock(
  mode: string,
  stamps: Record<string, string>,
): (() => string) | undefined {
  if (mode === 'fixed') return fixedNow(stamps.fixed ?? '');
  if (mode === 'missing') return undefined;
  if (mode !== 'advancing') assert.fail(`unknown clock mode ${mode}`);

  let calls = 0;
  return () => {
    const value =
      calls === 0
        ? (stamps.advancing_first ?? '')
        : (stamps.advancing_second ?? '');
    calls += 1;
    return value;
  };
}

function requireOne(
  items: readonly Record<string, AttributeValue>[],
): Record<string, AttributeValue> {
  const [first] = items;
  assert.ok(first !== undefined, 'the fake must have stored an item');
  assert.equal(items.length, 1, 'the fake must have stored exactly one item');
  return first;
}

async function capturePut(
  clock: (() => string) | undefined,
): Promise<Record<string, AttributeValue>> {
  const { client, fake } = createStatefulDynamoDBClient();
  const theorydb = new TheorydbClient(
    client,
    clock === undefined ? {} : { now: clock },
  ).register(LifecycleClocks);
  await theorydb.create('LifecycleClocks', { ...BASE_ITEM });
  return requireOne(fake.items(TABLE_NAME));
}

async function captureTransact(
  clock: (() => string) | undefined,
): Promise<Record<string, AttributeValue>> {
  const { client, fake } = createStatefulDynamoDBClient();
  const theorydb = new TheorydbClient(
    client,
    clock === undefined ? {} : { now: clock },
  ).register(LifecycleClocks);
  await theorydb.transactWrite([
    { kind: 'put', model: 'LifecycleClocks', item: { ...BASE_ITEM } },
  ]);
  return requireOne(fake.items(TABLE_NAME));
}

function stampToken(
  value: AttributeValue | undefined,
  stamps: Record<string, string>,
): string {
  if (value === undefined || !('S' in value)) return 'unstamped';
  const text = value.S ?? '';
  if (text === stamps.fixed) return 'stamp_fixed';
  if (text === stamps.advancing_first) return 'stamp_first';
  if (text === stamps.advancing_second) return 'stamp_second';
  if (text === '') return 'unstamped';
  return 'wall_clock';
}

function asWriterPath(path: string): ItemWriterPath {
  if (path !== 'put' && path !== 'transact_write') {
    assert.fail(`the shared table names an unknown writer path: ${path}`);
  }
  return path;
}

// The pin the shared table asks for: for every clock mode the ts column of both
// writer paths must match the captured payload, and the table's equality policy
// must match what the two paths really store.
void test('the shared lifecycle table pins the ts column on both writer paths', async () => {
  const table = loadLifecycleTable();

  assert.ok(table.runtimes.includes('ts'));
  assert.deepEqual(table.writer_paths, ['put', 'transact_write']);
  assert.deepEqual(table.clock_modes, ['fixed', 'advancing', 'missing']);
  assert.equal(table.rows.length, table.clock_modes.length);
  assert.ok(table.expectations.includes('wall_clock'));

  for (const row of table.rows) {
    const clock = scenarioClock(row.mode, table.stamps);
    const put = await capturePut(clock);
    const transact = await captureTransact(clock);

    assert.equal(
      stampToken(put['createdAt'], table.stamps),
      row.expect['put']?.['ts'],
      `put createdAt for the ${row.mode} clock`,
    );
    assert.equal(
      stampToken(transact['createdAt'], table.stamps),
      row.expect['transact_write']?.['ts'],
      `transact_write createdAt for the ${row.mode} clock`,
    );
    assert.deepEqual(
      put['createdAt'],
      put['updatedAt'],
      'both lifecycle fields of one put must come from the same clock read',
    );

    const equal = JSON.stringify(put) === JSON.stringify(transact);
    const expectedEquality = row.put_equals_transact_write?.['ts'];
    if (expectedEquality !== undefined) {
      assert.equal(
        expectedEquality,
        equal,
        `the ts writer paths store ${equal ? 'identical' : 'different'} bytes under the ${row.mode} clock`,
      );
    }

    // A preflight can only reproduce a stamped item when it is given the same
    // clock the writer read. Only the fixed row offers that.
    const supported = row.exact_equality_supported['ts'] === true;
    const preflightPut = marshalItemForWriterPath(
      LifecycleClocks,
      { ...BASE_ITEM },
      asWriterPath('put'),
      supported ? { now: table.stamps.fixed } : {},
    );
    if (supported) {
      assert.deepEqual(preflightPut, put);
      const preflightTransact = marshalItemForWriterPath(
        LifecycleClocks,
        { ...BASE_ITEM },
        asWriterPath('transact_write'),
        { now: table.stamps.fixed },
      );
      assert.deepEqual(preflightTransact, transact);
    }
  }
});
