import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';

import { decodeCursor, encodeCursor } from '../../src/cursor.js';
import { fromDynamoJson } from '../../src/dynamo-json.js';

const repoRoot = path.resolve(process.cwd(), '..');
const cursorDir = path.join(repoRoot, 'contract-tests/golden/cursor');
const jsonFiles = (await fs.readdir(cursorDir))
  .filter((name) => name.startsWith('cursor_v0.1_') && name.endsWith('.json'))
  .sort();

assert.ok(jsonFiles.length > 0);

for (const jsonFile of jsonFiles) {
  const stem = jsonFile.replace(/\.json$/, '');
  const cursorPath = path.join(cursorDir, `${stem}.cursor`);
  const jsonPath = path.join(cursorDir, jsonFile);

  const expectedEncoded = (await fs.readFile(cursorPath, 'utf8')).trim();
  const expectedJson = JSON.parse(await fs.readFile(jsonPath, 'utf8')) as {
    lastKey: Record<string, unknown>;
    index?: string;
    sort?: 'ASC' | 'DESC';
  };

  const lastKey = Object.fromEntries(
    Object.entries(expectedJson.lastKey).map(([key, value]) => [
      key,
      fromDynamoJson(value),
    ]),
  );
  assert.equal(
    encodeCursor({ ...expectedJson, lastKey }),
    expectedEncoded,
    stem,
  );
  if (expectedEncoded === '') continue;

  const decoded = decodeCursor(expectedEncoded);
  assert.deepEqual(decoded.lastKey, lastKey, stem);
  assert.equal(decoded.index, expectedJson.index, stem);
  assert.equal(decoded.sort, expectedJson.sort, stem);

  const reencoded = encodeCursor(decoded);
  assert.equal(reencoded, expectedEncoded, stem);
}
