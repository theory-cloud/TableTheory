import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

import type {
  DerivedKeyContract,
  DerivedKeyDefinition,
  DerivedKeyFixture,
  DerivedKeyInput,
  KeyContractInputValue,
} from '../ts/src/key-contract.js';

interface GeneratedModule {
  tabletheoryModelContract: DerivedKeyContract;
  [name: string]: unknown;
}

interface GoFixtureRow {
  key: string;
  fixture: string;
  expect: string;
  go: string;
}

main().catch((err: unknown) => {
  console.error(err);
  process.exit(1);
});

async function main(): Promise<void> {
  const [, , artifactArg, goMatrixArg] = process.argv;
  assert.ok(artifactArg, 'missing generated artifact path');
  assert.ok(goMatrixArg, 'missing Go fixture matrix path');

  const artifactPath = resolve(artifactArg);
  const goRows = JSON.parse(
    readFileSync(goMatrixArg, 'utf8'),
  ) as GoFixtureRow[];
  const goByFixture = new Map(
    goRows.map((row) => [`${row.key}\u0000${row.fixture}`, row]),
  );

  const generated = (await import(
    pathToFileURL(artifactPath).href
  )) as GeneratedModule;
  const contract = generated.tabletheoryModelContract;

  let checked = 0;
  for (const key of contract.derived_keys) {
    const helperName = key.helper ?? lowerCamel(key.name);
    const helper = generated[helperName];
    assert.equal(
      typeof helper,
      'function',
      `missing generated helper ${helperName} for ${key.name}`,
    );

    for (const fixture of key.fixtures ?? []) {
      const goRow = goByFixture.get(`${key.name}\u0000${fixture.name}`);
      assert.ok(
        goRow,
        `missing Go evaluator row for ${key.name}/${fixture.name}`,
      );
      assert.equal(
        goRow.expect,
        fixture.expect,
        `Go matrix expectation drift for ${key.name}/${fixture.name}`,
      );
      assert.equal(
        goRow.go,
        fixture.expect,
        `Go evaluator drift for ${key.name}/${fixture.name}`,
      );

      const got = (
        helper as (input: Record<string, KeyContractInputValue>) => string
      )(helperInput(key, fixture));
      assert.equal(
        got,
        fixture.expect,
        `generated helper drift for ${key.name}/${fixture.name}`,
      );
      assert.equal(
        got,
        goRow.go,
        `generated helper vs Go evaluator drift for ${key.name}/${fixture.name}`,
      );
      checked += 1;
    }
  }

  assert.equal(
    checked,
    goRows.length,
    'generated helper fixture count must match Go evaluator rows',
  );
  console.log(`generated-ts: PASS (${checked} fixtures)`);
}

function helperInput(
  key: DerivedKeyDefinition,
  fixture: DerivedKeyFixture,
): Record<string, KeyContractInputValue> {
  const out: Record<string, KeyContractInputValue> = {};
  for (const input of key.inputs ?? []) {
    if (Object.prototype.hasOwnProperty.call(fixture.input, input.name)) {
      out[tsPropertyName(input)] = fixture.input[input.name];
    }
  }
  return out;
}

function tsPropertyName(input: DerivedKeyInput): string {
  return input.ts_name ?? lowerCamel(input.name);
}

const splitNamePattern = /[^A-Za-z0-9]+/u;

function lowerCamel(value: string): string {
  const parts = value.split(splitNamePattern).filter((part) => part !== '');
  if (parts.length === 0) return '';
  return parts
    .map((part, index) => {
      const lowered = part.toLowerCase();
      return index === 0 ? lowered : upperFirst(lowered);
    })
    .join('');
}

function upperFirst(value: string): string {
  if (value === '') return '';
  return value.slice(0, 1).toUpperCase() + value.slice(1);
}
