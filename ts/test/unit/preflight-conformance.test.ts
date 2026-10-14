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

const NOW = '2026-01-16T00:00:00.123000000Z';
const TABLE_NAME = 'preflight_conformance';

// The shared, language-neutral table the Go, TypeScript and Python suites all
// consume. It is resolved from this file's own URL so the path holds however
// the suite is launched.
const CONFORMANCE_TABLE_PATH = new URL(
  '../../../contract-tests/serialization/nil-empty-collections.yml',
  import.meta.url,
);

type ConformanceRow = {
  field: string;
  container: string;
  omitempty: boolean;
  expect: Record<string, Record<string, string>>;
};

type ConformanceTable = {
  version: number;
  writer_paths: string[];
  runtimes: string[];
  representations: string[];
  rows: ConformanceRow[];
};

function loadConformanceTable(): ConformanceTable {
  assert.ok(
    existsSync(CONFORMANCE_TABLE_PATH),
    `the shared conformance table must exist at ${CONFORMANCE_TABLE_PATH.pathname}`,
  );

  const table = parse(
    readFileSync(CONFORMANCE_TABLE_PATH, 'utf8'),
  ) as ConformanceTable;

  assert.equal(
    table.version,
    1,
    'the shared conformance table must be version 1',
  );
  assert.ok(
    Array.isArray(table.rows) && table.rows.length > 0,
    'the shared conformance table must declare rows',
  );
  return table;
}

// The model the shared table's rows are mapped onto. TypeScript has no nil
// collection: a collection value is an array or an object, so its nearest input
// to Go's nil slice is an absent property, which marshalPutItem drops instead
// of storing as NULL. A nil collection nested inside an M attribute is
// expressed as an explicit null (an undefined nested value throws), and an
// empty SS attribute is stored as NULL.
const ConformanceMatrix = defineModel({
  name: 'PreflightConformance',
  table: { name: TABLE_NAME },
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
    { attribute: 'emptySet', type: 'SS', optional: true },
    { attribute: 'nested', type: 'M', optional: true },
  ],
});

const BASE_ITEM: Record<string, unknown> = { PK: 'P#1', SK: 'S#1' };

type RowInput = {
  item: Record<string, unknown>;
  read: (item: Record<string, AttributeValue>) => AttributeValue | undefined;
};

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

// Maps each logical field in the shared table to the TypeScript attribute it is
// stored as, plus the input that produces that row's representation. A nil
// collection is an absent property, so it shares its attribute with the empty
// row of the same shape and only the input differs.
function conformanceRowInputs(): Record<string, RowInput> {
  return {
    nil_list: { item: BASE_ITEM, read: rootReader('rootList') },
    empty_list: {
      item: { ...BASE_ITEM, rootList: [] },
      read: rootReader('rootList'),
    },
    nil_map: { item: BASE_ITEM, read: rootReader('rootMap') },
    empty_map: {
      item: { ...BASE_ITEM, rootMap: {} },
      read: rootReader('rootMap'),
    },
    nil_list_omit: { item: BASE_ITEM, read: rootReader('rootListOmit') },
    empty_list_omit: {
      item: { ...BASE_ITEM, rootListOmit: [] },
      read: rootReader('rootListOmit'),
    },
    nil_map_omit: { item: BASE_ITEM, read: rootReader('rootMapOmit') },
    empty_map_omit: {
      item: { ...BASE_ITEM, rootMapOmit: {} },
      read: rootReader('rootMapOmit'),
    },
    false_bool: {
      item: { ...BASE_ITEM, flag: false },
      read: rootReader('flag'),
    },
    zero_number: {
      item: { ...BASE_ITEM, score: 0 },
      read: rootReader('score'),
    },
    empty_set: {
      item: { ...BASE_ITEM, emptySet: [] },
      read: rootReader('emptySet'),
    },
    nested_nil_list: {
      item: { ...BASE_ITEM, nested: { nestedNilList: null } },
      read: nestedReader('nestedNilList'),
    },
    nested_empty_list: {
      item: { ...BASE_ITEM, nested: { nestedEmptyList: [] } },
      read: nestedReader('nestedEmptyList'),
    },
    nested_nil_map: {
      item: { ...BASE_ITEM, nested: { nestedNilMap: null } },
      read: nestedReader('nestedNilMap'),
    },
    nested_empty_map: {
      item: { ...BASE_ITEM, nested: { nestedEmptyMap: {} } },
      read: nestedReader('nestedEmptyMap'),
    },
  };
}

function asWriterPath(path: string): ItemWriterPath {
  if (path !== 'put' && path !== 'transact_write') {
    assert.fail(`the shared table names an unknown writer path: ${path}`);
  }
  return path;
}

function representation(av: AttributeValue | undefined): string {
  if (av === undefined) return 'ABSENT';
  if ('NULL' in av && av.NULL) return 'NULL';
  if ('L' in av) return 'L';
  if ('M' in av) return 'M';
  if ('S' in av) return 'S';
  if ('N' in av) return 'N';
  if ('BOOL' in av) return 'BOOL';
  return 'OTHER';
}

function requireOne(
  items: readonly Record<string, AttributeValue>[],
  message: string,
): Record<string, AttributeValue> {
  const [first] = items;
  assert.ok(first !== undefined, message);
  assert.equal(items.length, 1, message);
  return first;
}

async function capturePut(
  item: Record<string, unknown>,
): Promise<Record<string, AttributeValue>> {
  const { client, fake } = createStatefulDynamoDBClient();
  const theorydb = new TheorydbClient(client, { now: fixedNow(NOW) }).register(
    ConformanceMatrix,
  );
  await theorydb.create('PreflightConformance', item);
  return requireOne(
    fake.items(TABLE_NAME),
    'the fake must have stored exactly one put item',
  );
}

async function captureTransactWritePut(
  item: Record<string, unknown>,
): Promise<Record<string, AttributeValue>> {
  const { client, fake } = createStatefulDynamoDBClient();
  const theorydb = new TheorydbClient(client, { now: fixedNow(NOW) }).register(
    ConformanceMatrix,
  );
  await theorydb.transactWrite([
    { kind: 'put', model: 'PreflightConformance', item },
  ]);
  return requireOne(
    fake.items(TABLE_NAME),
    'the fake must have stored exactly one transacted item',
  );
}

const CAPTURES: Record<
  ItemWriterPath,
  (item: Record<string, unknown>) => Promise<Record<string, AttributeValue>>
> = {
  put: capturePut,
  transact_write: captureTransactWritePut,
};

// The pin the shared table asks for: for every row and every writer path the
// captured payload's representation must equal the table's ts column, and the
// preflight answer must equal the capture. Every row is mapped and every cell
// counted, so a dropped or renamed row fails the build instead of silently
// shrinking the pin.
void test('the shared conformance table pins the ts column on both writer paths', async () => {
  const table = loadConformanceTable();
  const inputs = conformanceRowInputs();

  assert.ok(
    table.runtimes.includes('ts'),
    'the shared conformance table must list the ts runtime',
  );
  assert.equal(
    table.rows.length,
    Object.keys(inputs).length,
    'the shared table must declare exactly the fields the TypeScript runtime maps',
  );

  const seen = new Set<string>();
  let capturedCells = 0;
  let preflightCells = 0;

  for (const row of table.rows) {
    const input = inputs[row.field];
    assert.ok(
      input !== undefined,
      `the shared table names a field the TypeScript runtime does not map: ${row.field}`,
    );
    assert.ok(
      !seen.has(row.field),
      `the shared table repeats the field ${row.field}`,
    );
    seen.add(row.field);

    for (const rawPath of table.writer_paths) {
      const path = asWriterPath(rawPath);

      const expectations = row.expect[rawPath];
      assert.ok(
        expectations !== undefined,
        `row ${row.field} declares no expectation for writer path ${rawPath}`,
      );
      const want = expectations['ts'];
      assert.ok(
        want !== undefined,
        `row ${row.field} declares no ts expectation for writer path ${rawPath}`,
      );
      assert.ok(
        table.representations.includes(want),
        `row ${row.field} expects an unknown representation ${want}`,
      );

      const captured = await CAPTURES[path](input.item);
      assert.equal(
        representation(input.read(captured)),
        want,
        `captured ${rawPath} payload for ${row.field}`,
      );

      const preflight = marshalItemForWriterPath(
        ConformanceMatrix,
        input.item,
        path,
      );
      assert.equal(
        representation(input.read(preflight)),
        want,
        `preflight ${rawPath} answer for ${row.field}`,
      );

      capturedCells += 1;
      preflightCells += 1;
    }
  }

  const derivedCells = table.rows.length * table.writer_paths.length;
  assert.equal(
    capturedCells,
    derivedCells,
    'every row/writer-path cell must be checked against a capture',
  );
  assert.equal(
    preflightCells,
    derivedCells,
    'every row/writer-path cell must be checked against the preflight',
  );
});
